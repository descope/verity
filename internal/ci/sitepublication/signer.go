package sitepublication

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/verity-org/verity/internal/ci/publication"
	"github.com/verity-org/verity/internal/ci/signerlock"
)

const (
	trustedDockerBinary  = "/usr/bin/docker"
	trustedPodmanBinary  = "/usr/bin/podman"
	trustedGHBinary      = "/usr/bin/gh"
	trustedMelangeBinary = "/usr/bin/melange"
	trustedSignerBinary  = "/usr/bin/apk-repository-signer"
	trustedSignerRepo    = "verity-org/verity"
	signerInputRoot      = "/inputs"
	signerOutputPath     = "/output"
	signerKeyDirectory   = "/run/verity-signing"
	signerTmpfsArgument  = signerKeyDirectory + ":rw,nosuid,nodev,noexec,size=64k,mode=0700,uid=65532,gid=65532"
)

var signerRepositoryPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

func BuildSignerPlan(request *SignerRequest) (SignerPlan, error) {
	if request == nil {
		return SignerPlan{}, fmt.Errorf("%w: request is required", ErrInvalidSignerPlan)
	}
	if err := validatePlan(&request.Plan); err != nil {
		return SignerPlan{}, err
	}
	runtime, err := parseContainerRuntime(request.Runtime)
	if err != nil {
		return SignerPlan{}, err
	}
	workspace, keyDirectory, err := validateSignerDirectories(request.WorkspaceDir, request.KeyDirectory)
	if err != nil {
		return SignerPlan{}, err
	}
	spec, err := newSignerExecutionSpec(request, runtime, workspace)
	if err != nil {
		return SignerPlan{}, err
	}
	plan := SignerPlan{
		SchemaVersion: SchemaVersion, PublicationPlanDigest: request.Plan.PlanDigest,
		ManifestDigest: request.Plan.ManifestDigest, SignerDigest: request.Plan.SignerDigest,
		SignerSourceSHA: request.Plan.SignerSourceSHA, ImageReference: request.Plan.SignerReference,
		Execution: spec,
		Cleanup:   KeyCleanup{KeyDirectory: keyDirectory, KeyPath: filepath.Join(keyDirectory, "verity.rsa")},
	}
	if err := bindSignerFilesystem(&plan); err != nil {
		return SignerPlan{}, err
	}
	plan.Authorization, plan.InputDigest, err = buildSignerAuthorization(&request.Plan, &plan.Execution)
	if err != nil {
		return SignerPlan{}, err
	}
	plan.Steps, err = buildSignerSteps(&plan)
	if err != nil {
		return SignerPlan{}, err
	}
	if err := ValidateSignerPlan(&plan); err != nil {
		return SignerPlan{}, err
	}
	return plan, nil
}

func newSignerExecutionSpec(request *SignerRequest, runtime ContainerRuntime, workspace string) (SignerExecutionSpec, error) {
	if err := validateSignerRepository(request.Repository); err != nil {
		return SignerExecutionSpec{}, err
	}
	manifest, err := cleanSignerPath(request.ManifestPath)
	if err != nil {
		return SignerExecutionSpec{}, err
	}
	packages, err := cleanSignerPath(request.PackagesPath)
	if err != nil {
		return SignerExecutionSpec{}, err
	}
	output, err := cleanSignerPath(request.OutputAPKPath)
	if err != nil {
		return SignerExecutionSpec{}, err
	}
	publicKey, err := cleanSignerPath(request.PublicKeyPath)
	if err != nil {
		return SignerExecutionSpec{}, err
	}
	spec := SignerExecutionSpec{
		Runtime: runtime, Mode: request.Plan.Mode, Repository: request.Repository, WorkspaceDir: workspace,
		ManifestPath: manifest, PackagesPath: packages, OutputAPKPath: output, PublicKeyPath: publicKey,
	}
	switch request.Plan.Mode {
	case publication.ModeBootstrap, publication.ModeSnapshot:
	case publication.ModeDelta:
		spec.BaseAPKPath, err = cleanSignerPath(request.BaseAPKPath)
		if err != nil {
			return SignerExecutionSpec{}, err
		}
		spec.DeltaManifestPath, err = cleanSignerPath(request.DeltaManifestPath)
		if err != nil {
			return SignerExecutionSpec{}, err
		}
	case publication.ModeRestore:
		return SignerExecutionSpec{}, ErrUnsupportedSignMode
	default:
		return SignerExecutionSpec{}, fmt.Errorf("%w: mode %q", ErrInvalidSignerPlan, request.Plan.Mode)
	}
	return spec, nil
}

func buildSignerSteps(plan *SignerPlan) ([]SignerStep, error) {
	runtime, err := trustedRuntimeBinary(plan.Execution.Runtime)
	if err != nil {
		return nil, err
	}
	runArgs := make([]string, 0, 24)
	runArgs = append(
		runArgs,
		"run", "--rm", "--interactive", "--network=none", "--read-only", "--user=65532:65532",
		"--cap-drop=ALL", "--security-opt=no-new-privileges", "--pids-limit=256",
		"--workdir=/", "--tmpfs="+signerTmpfsArgument, "--env=TMPDIR="+signerKeyDirectory,
	)
	runArgs = append(runArgs, signerMountArguments(plan)...)
	spec := &plan.Execution
	authorization, err := signerAuthorizationBase64(&plan.Authorization)
	if err != nil {
		return nil, err
	}
	signArgs := append(
		append([]string(nil), runArgs...),
		"--entrypoint="+trustedSignerBinary, plan.ImageReference,
		"ci", "site-publication", "sign",
		"--publication-plan-digest="+string(plan.PublicationPlanDigest),
		"--manifest-digest="+string(plan.ManifestDigest),
		"--input-digest="+string(plan.InputDigest),
		"--input-authorization="+authorization,
		"--input-root="+signerInputRoot,
		"--packages", signerInputPath(spec.PackagesPath),
		"--output", signerOutputPath,
		"--public-key", signerInputPath(spec.PublicKeyPath),
	)
	if spec.Mode == publication.ModeDelta {
		signArgs = append(
			signArgs,
			"--base-apk", signerInputPath(spec.BaseAPKPath),
			"--delta-manifest", signerInputPath(spec.DeltaManifestPath),
		)
	}
	signArgs = append(signArgs, signerInputPath(spec.ManifestPath))
	return []SignerStep{
		{Name: "pull", Command: ExecutionCommand{Name: runtime, Args: []string{"pull", plan.ImageReference}}},
		{Name: "attest", Command: ExecutionCommand{Name: trustedGHBinary, Args: []string{
			"attestation", "verify", "oci://" + plan.ImageReference,
			"--repo", plan.Execution.Repository,
			"--signer-workflow", signerlock.TrustedWorkflowIdentity,
			"--source-ref", "refs/heads/main", "--source-digest", string(plan.SignerSourceSHA),
			"--deny-self-hosted-runners",
		}}},
		{Name: "sign", KeyAccess: true, Command: ExecutionCommand{Name: runtime, Args: signArgs}},
	}, nil
}

type signerMount struct {
	source      string
	destination string
	readonly    bool
}

func signerMountArguments(plan *SignerPlan) []string {
	spec := plan.Execution
	mounts := []signerMount{
		{source: filepath.Join(spec.WorkspaceDir, filepath.FromSlash(spec.ManifestPath)), destination: signerInputPath(spec.ManifestPath), readonly: true},
		{source: filepath.Join(spec.WorkspaceDir, filepath.FromSlash(spec.PackagesPath)), destination: signerInputPath(spec.PackagesPath), readonly: true},
		{source: filepath.Join(spec.WorkspaceDir, filepath.FromSlash(spec.PublicKeyPath)), destination: signerInputPath(spec.PublicKeyPath), readonly: true},
		{source: filepath.Join(spec.WorkspaceDir, filepath.FromSlash(spec.OutputAPKPath)), destination: signerOutputPath},
	}
	if spec.Mode == publication.ModeDelta {
		mounts = append(
			mounts,
			signerMount{source: filepath.Join(spec.WorkspaceDir, filepath.FromSlash(spec.BaseAPKPath)), destination: signerInputPath(spec.BaseAPKPath), readonly: true},
			signerMount{source: filepath.Join(spec.WorkspaceDir, filepath.FromSlash(spec.DeltaManifestPath)), destination: signerInputPath(spec.DeltaManifestPath), readonly: true},
		)
	}
	arguments := make([]string, 0, len(mounts))
	for _, mount := range mounts {
		argument := "--mount=type=bind,src=" + mount.source + ",dst=" + mount.destination
		if mount.readonly {
			argument += ",readonly"
		}
		arguments = append(arguments, argument)
	}
	return arguments
}

func signerInputPath(relative string) string {
	return signerInputRoot + "/" + relative
}

func parseContainerRuntime(value string) (ContainerRuntime, error) {
	switch ContainerRuntime(value) {
	case ContainerRuntimeDocker:
		return ContainerRuntimeDocker, nil
	case ContainerRuntimePodman:
		return ContainerRuntimePodman, nil
	default:
		return "", fmt.Errorf("%w: runtime %q", ErrInvalidSignerPlan, value)
	}
}

func trustedRuntimeBinary(runtime ContainerRuntime) (string, error) {
	switch runtime {
	case ContainerRuntimeDocker:
		return trustedDockerBinary, nil
	case ContainerRuntimePodman:
		return trustedPodmanBinary, nil
	default:
		return "", fmt.Errorf("%w: runtime %q", ErrInvalidSignerPlan, runtime)
	}
}

func validateSignerRepository(repository string) error {
	if repository != trustedSignerRepo || !signerRepositoryPattern.MatchString(repository) {
		return fmt.Errorf("%w: repository", ErrInvalidSignerPlan)
	}
	return nil
}

func cleanSignerPath(value string) (string, error) {
	if strings.Contains(value, ",") {
		return "", fmt.Errorf("%w: unsafe path %q", ErrInvalidSignerPlan, value)
	}
	clean, err := safeRelative(filepath.ToSlash(value))
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrInvalidSignerPlan, err)
	}
	return clean, nil
}

func validateSignerDirectories(workspace, keyDirectory string) (cleanWorkspace, cleanKeyDirectory string, resultErr error) {
	cleanWorkspace = filepath.Clean(workspace)
	cleanKeyDirectory = filepath.Clean(keyDirectory)
	if workspace != cleanWorkspace || keyDirectory != cleanKeyDirectory || !filepath.IsAbs(cleanWorkspace) ||
		!filepath.IsAbs(cleanKeyDirectory) || strings.Contains(cleanWorkspace, ",") || strings.Contains(cleanKeyDirectory, ",") {
		return "", "", fmt.Errorf("%w: signer directories must be canonical absolute mount-safe paths", ErrInvalidSignerPlan)
	}
	if insideDirectory(cleanWorkspace, cleanKeyDirectory) || cleanWorkspace == cleanKeyDirectory {
		return "", "", fmt.Errorf("%w: key directory must be outside workspace", ErrInvalidSignerPlan)
	}
	return cleanWorkspace, cleanKeyDirectory, nil
}

func insideDirectory(directory, candidate string) bool {
	relative, err := filepath.Rel(directory, candidate)
	return err == nil && relative != ".." && relative != "." && !filepath.IsAbs(relative) &&
		!strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
