package runson

import (
	"context"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errUnexpectedTestCommand = errors.New("unexpected test command")

func TestVerifier_Verify_acceptsCompliantHost(t *testing.T) {
	// Given a RunsOn host with the expected identity, capacity, Docker, and encrypted EBS.
	metadata := newMetadataServer(t)
	verifier := verifierFixture(t, metadata.URL, true)
	requirements, err := NewRequirements(RequirementInput{
		ExpectedAccount:  "123456789012",
		ExpectedRegion:   "us-east-1",
		ExpectedArch:     "amd64",
		MinimumCPU:       4,
		MinimumMemoryGiB: 7,
		MinimumDiskGiB:   30,
	})
	require.NoError(t, err)

	// When the host is verified.
	report, err := verifier.Verify(context.Background(), requirements)

	// Then the report binds the observed AWS and runner identity.
	require.NoError(t, err)
	assert.Equal(t, "i-0123456789abcdef0", report.InstanceID)
	assert.Equal(t, "arn:aws:sts::123456789012:assumed-role/runs-on/i-0123456789abcdef0", report.CallerARN)
	assert.Equal(t, "27.5.1", report.DockerVersion)
	assert.True(t, report.AllVolumesEncrypted)
}

func TestVerifier_Verify_rejectsUnencryptedVolume(t *testing.T) {
	// Given a RunsOn host with one unencrypted attached EBS volume.
	metadata := newMetadataServer(t)
	verifier := verifierFixture(t, metadata.URL, false)
	requirements, err := NewRequirements(RequirementInput{
		ExpectedAccount:  "123456789012",
		ExpectedRegion:   "us-east-1",
		ExpectedArch:     "amd64",
		MinimumCPU:       4,
		MinimumMemoryGiB: 7,
		MinimumDiskGiB:   30,
	})
	require.NoError(t, err)

	// When the host is verified.
	_, err = verifier.Verify(context.Background(), requirements)

	// Then encrypted storage remains mandatory.
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrVerificationFailed)
}

func TestNewRequirements_rejectsInvalidAWSAccount(t *testing.T) {
	// Given an invalid account identifier.
	input := RequirementInput{
		ExpectedAccount:  "not-an-account",
		ExpectedRegion:   "us-east-1",
		ExpectedArch:     "amd64",
		MinimumCPU:       4,
		MinimumMemoryGiB: 7,
		MinimumDiskGiB:   30,
	}

	// When requirements are parsed.
	_, err := NewRequirements(input)

	// Then invalid boundary input is rejected.
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidRequirements)
}

func TestNewRequirements_rejectsCapacityOverflow(t *testing.T) {
	// Given a capacity value that cannot be represented in bytes.
	input := RequirementInput{
		ExpectedAccount:  "123456789012",
		ExpectedRegion:   "us-east-1",
		ExpectedArch:     "amd64",
		MinimumCPU:       4,
		MinimumMemoryGiB: math.MaxInt,
		MinimumDiskGiB:   30,
	}

	// When requirements are parsed.
	_, err := NewRequirements(input)

	// Then integer overflow is rejected at the boundary.
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidRequirements)
}

func newMetadataServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/latest/api/token":
			require.Equal(t, http.MethodPut, request.Method)
			_, err := writer.Write([]byte("sentinel-token"))
			require.NoError(t, err)
		case "/latest/dynamic/instance-identity/document":
			require.Equal(t, "sentinel-token", request.Header.Get("X-aws-ec2-metadata-token"))
			_, err := writer.Write([]byte(`{"accountId":"123456789012","region":"us-east-1","instanceId":"i-0123456789abcdef0"}`))
			require.NoError(t, err)
		default:
			http.NotFound(writer, request)
		}
	}))
}

func verifierFixture(t *testing.T, metadataEndpoint string, encrypted bool) Verifier {
	t.Helper()
	return Verifier{
		metadataEndpoint: metadataEndpoint,
		client:           http.DefaultClient,
		getenv: func(name string) string {
			if name == "RUNS_ON_RUNNER_NAME" {
				return "runs-on-verity-canary"
			}
			return ""
		},
		architecture: func() string { return "amd64" },
		cpuCount:     func() int { return 4 },
		memoryBytes:  func() (uint64, error) { return 8 * gibibyte, nil },
		diskBytes:    func() (uint64, error) { return 40 * gibibyte, nil },
		execute: func(_ context.Context, name string, arguments ...string) ([]byte, error) {
			switch name {
			case "docker":
				require.Equal(t, []string{"info", "--format", "{{.ServerVersion}}"}, arguments)
				return []byte("27.5.1\n"), nil
			case "aws":
				if len(arguments) > 1 && arguments[0] == "sts" {
					require.Equal(t, []string{"sts", "get-caller-identity", "--region", "us-east-1", "--output", "json"}, arguments)
					return []byte(`{"Account":"123456789012","Arn":"arn:aws:sts::123456789012:assumed-role/runs-on/i-0123456789abcdef0"}`), nil
				}
				if len(arguments) > 1 && arguments[0] == "ec2" {
					require.Equal(t, []string{
						"ec2", "describe-volumes", "--filters", "Name=attachment.instance-id,Values=i-0123456789abcdef0",
						"--region", "us-east-1", "--output", "json",
					}, arguments)
					return []byte(`{"Volumes":[{"VolumeId":"vol-0123456789abcdef0","Encrypted":` + strconv.FormatBool(encrypted) + `}]}`), nil
				}
			}
			return nil, errUnexpectedTestCommand
		},
	}
}
