package runson

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

const gibibyte = uint64(1024 * 1024 * 1024)

var (
	ErrInvalidRequirements = errors.New("invalid RunsOn verification requirements")
	ErrVerificationFailed  = errors.New("RunsOn host verification failed")
	awsAccountPattern      = regexp.MustCompile(`^\d{12}$`)
	awsRegionPattern       = regexp.MustCompile(`^[a-z]{2}(?:-gov)?-[a-z]+-\d+$`)
)

type RequirementInput struct {
	ExpectedAccount  string
	ExpectedRegion   string
	ExpectedArch     string
	MinimumCPU       int
	MinimumMemoryGiB int
	MinimumDiskGiB   int
}

type Requirements struct {
	expectedAccount   string
	expectedRegion    string
	expectedArch      string
	minimumCPU        int
	minimumMemoryByte uint64
	minimumDiskByte   uint64
}

type Report struct {
	RunnerName          string `json:"runner_name"`
	Account             string `json:"account"`
	Region              string `json:"region"`
	InstanceID          string `json:"instance_id"`
	CallerARN           string `json:"caller_arn"`
	Architecture        string `json:"architecture"`
	CPUCount            int    `json:"cpu_count"`
	MemoryBytes         uint64 `json:"memory_bytes"`
	RootDiskBytes       uint64 `json:"root_disk_bytes"`
	DockerVersion       string `json:"docker_version"`
	AllVolumesEncrypted bool   `json:"all_volumes_encrypted"`
}

type Verifier struct {
	metadataEndpoint string
	client           *http.Client
	getenv           func(string) string
	architecture     func() string
	cpuCount         func() int
	memoryBytes      func() (uint64, error)
	diskBytes        func() (uint64, error)
	execute          func(context.Context, string, ...string) ([]byte, error)
}

type instanceIdentity struct {
	AccountID  string `json:"accountId"`
	Region     string `json:"region"`
	InstanceID string `json:"instanceId"`
}

type callerIdentity struct {
	Account string `json:"Account"`
	ARN     string `json:"Arn"`
}

type volumeList struct {
	Volumes []volume `json:"Volumes"`
}

type volume struct {
	ID        string `json:"VolumeId"`
	Encrypted bool   `json:"Encrypted"`
}

type hostCapacity struct {
	architecture string
	cpuCount     int
	memoryBytes  uint64
	diskBytes    uint64
}

func NewRequirements(input RequirementInput) (Requirements, error) {
	account := strings.TrimSpace(input.ExpectedAccount)
	region := strings.TrimSpace(input.ExpectedRegion)
	arch := strings.TrimSpace(input.ExpectedArch)
	if !awsAccountPattern.MatchString(account) || !awsRegionPattern.MatchString(region) {
		return Requirements{}, ErrInvalidRequirements
	}
	if arch != "amd64" && arch != "arm64" {
		return Requirements{}, ErrInvalidRequirements
	}
	if input.MinimumCPU < 1 || input.MinimumMemoryGiB < 1 || input.MinimumDiskGiB < 1 {
		return Requirements{}, ErrInvalidRequirements
	}
	maximumGiB := int(^uint64(0) / gibibyte)
	if input.MinimumMemoryGiB > maximumGiB || input.MinimumDiskGiB > maximumGiB {
		return Requirements{}, ErrInvalidRequirements
	}
	return Requirements{
		expectedAccount:   account,
		expectedRegion:    region,
		expectedArch:      arch,
		minimumCPU:        input.MinimumCPU,
		minimumMemoryByte: uint64(input.MinimumMemoryGiB) * gibibyte,
		minimumDiskByte:   uint64(input.MinimumDiskGiB) * gibibyte,
	}, nil
}

func NewVerifier() Verifier {
	return systemVerifier()
}

func (v Verifier) Verify(ctx context.Context, requirements Requirements) (Report, error) {
	runnerName := strings.TrimSpace(v.getenv("RUNS_ON_RUNNER_NAME"))
	if runnerName == "" {
		return Report{}, verificationError("runner identity", "RUNS_ON_RUNNER_NAME is empty")
	}
	capacity, err := v.observeCapacity()
	if err != nil {
		return Report{}, err
	}
	if err := capacity.validate(requirements); err != nil {
		return Report{}, err
	}
	identity, err := v.readInstanceIdentity(ctx)
	if err != nil {
		return Report{}, err
	}
	if identity.AccountID != requirements.expectedAccount || identity.Region != requirements.expectedRegion {
		return Report{}, verificationError("AWS identity", "account or region does not match the requested canary target")
	}

	caller, allEncrypted, err := v.verifyAWS(ctx, identity, requirements.expectedAccount)
	if err != nil {
		return Report{}, err
	}
	dockerOutput, err := v.execute(ctx, "docker", "info", "--format", "{{.ServerVersion}}")
	if err != nil {
		return Report{}, verificationError("Docker", err.Error())
	}
	dockerVersion := strings.TrimSpace(string(dockerOutput))
	if dockerVersion == "" {
		return Report{}, verificationError("Docker", "server version is empty")
	}

	return Report{
		RunnerName:          runnerName,
		Account:             identity.AccountID,
		Region:              identity.Region,
		InstanceID:          identity.InstanceID,
		CallerARN:           caller.ARN,
		Architecture:        capacity.architecture,
		CPUCount:            capacity.cpuCount,
		MemoryBytes:         capacity.memoryBytes,
		RootDiskBytes:       capacity.diskBytes,
		DockerVersion:       dockerVersion,
		AllVolumesEncrypted: allEncrypted,
	}, nil
}

func (v Verifier) observeCapacity() (hostCapacity, error) {
	memoryBytes, err := v.memoryBytes()
	if err != nil {
		return hostCapacity{}, verificationError("memory", err.Error())
	}
	diskBytes, err := v.diskBytes()
	if err != nil {
		return hostCapacity{}, verificationError("root disk", err.Error())
	}
	return hostCapacity{
		architecture: v.architecture(),
		cpuCount:     v.cpuCount(),
		memoryBytes:  memoryBytes,
		diskBytes:    diskBytes,
	}, nil
}

func (capacity hostCapacity) validate(requirements Requirements) error {
	if capacity.architecture != requirements.expectedArch || capacity.cpuCount < requirements.minimumCPU ||
		capacity.memoryBytes < requirements.minimumMemoryByte || capacity.diskBytes < requirements.minimumDiskByte {
		return verificationError("capacity", "architecture, CPU, memory, or disk is below the required profile")
	}
	return nil
}

func (v Verifier) verifyAWS(
	ctx context.Context,
	identity instanceIdentity,
	expectedAccount string,
) (callerIdentity, bool, error) {
	caller, err := v.readCallerIdentity(ctx, identity.Region)
	if err != nil {
		return callerIdentity{}, false, err
	}
	if caller.Account != expectedAccount {
		return callerIdentity{}, false, verificationError("AWS caller", "STS account does not match the instance identity")
	}
	allEncrypted, err := v.verifyVolumes(ctx, identity)
	if err != nil {
		return callerIdentity{}, false, err
	}
	return caller, allEncrypted, nil
}

func (v Verifier) readCallerIdentity(ctx context.Context, region string) (callerIdentity, error) {
	output, err := v.execute(ctx, "aws", "sts", "get-caller-identity", "--region", region, "--output", "json")
	if err != nil {
		return callerIdentity{}, verificationError("AWS caller", err.Error())
	}
	var identity callerIdentity
	if err := json.Unmarshal(output, &identity); err != nil || identity.Account == "" || identity.ARN == "" {
		return callerIdentity{}, verificationError("AWS caller", "invalid STS identity response")
	}
	return identity, nil
}

func (v Verifier) verifyVolumes(ctx context.Context, identity instanceIdentity) (bool, error) {
	output, err := v.execute(ctx, "aws", "ec2", "describe-volumes", "--filters",
		"Name=attachment.instance-id,Values="+identity.InstanceID, "--region", identity.Region, "--output", "json")
	if err != nil {
		return false, verificationError("EBS encryption", err.Error())
	}
	var volumes volumeList
	if err := json.Unmarshal(output, &volumes); err != nil || len(volumes.Volumes) == 0 {
		return false, verificationError("EBS encryption", "no attached EBS volumes were returned")
	}
	for _, attached := range volumes.Volumes {
		if attached.ID == "" || !attached.Encrypted {
			return false, verificationError("EBS encryption", "an attached volume is unencrypted or unidentified")
		}
	}
	return true, nil
}

func verificationError(check, detail string) error {
	return fmt.Errorf("%s: %s: %w", check, detail, ErrVerificationFailed)
}
