package repositoryops

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	veritypatch "github.com/verity-org/verity/internal/patch"
)

const patchTimeout = 30 * time.Minute

var digestPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

type patchPackageMode string

const (
	patchPackageModeFull   patchPackageMode = "os,library"
	patchPackageModeOSOnly patchPackageMode = "os"
)

type PatchSpec struct {
	Source              string
	Destination         string
	Report              string
	PackageTypes        string
	LibraryPatchLevel   string
	ToolchainPatchLevel string
	BuildKitAddress     string
	GoVCSURL            string
	Timeout             time.Duration
	Push                bool
}

type Patcher interface {
	Patch(context.Context, *PatchSpec) error
}

type CopaPatcher struct{}

func (CopaPatcher) Patch(ctx context.Context, spec *PatchSpec) error {
	return veritypatch.Run(ctx, &veritypatch.Config{
		Image: spec.Source, PatchedTag: spec.Destination, Report: spec.Report,
		PkgTypes: spec.PackageTypes, LibraryPatchLevel: spec.LibraryPatchLevel,
		ToolchainPatchLevel: spec.ToolchainPatchLevel, Push: spec.Push,
		BuildKitAddr: spec.BuildKitAddress, Timeout: spec.Timeout, GoVCSURL: spec.GoVCSURL,
	})
}

type PatchService struct {
	Patcher  Patcher
	Commands CommandRunner
}

type PatchResult struct {
	Destination    string
	Digest         string
	CopiedSource   bool
	RetriedOSOnly  bool
	AlreadyCurrent bool
}

func (s PatchService) Run(ctx context.Context, request *PatchRequest) (PatchResult, error) {
	if request == nil {
		return PatchResult{}, fmt.Errorf("%w: patch request is required", ErrInvalidPatchRequest)
	}
	patcher := s.Patcher
	if patcher == nil {
		patcher = CopaPatcher{}
	}
	commands := s.Commands
	if commands == nil {
		commands = ExecCommandRunner{}
	}
	outcome, err := runPatchPolicy(ctx, patcher, request)
	if err != nil {
		return PatchResult{}, err
	}
	digest, copiedSource, err := verifyStagingImage(ctx, commands, request)
	if err != nil {
		return PatchResult{}, err
	}
	return PatchResult{
		Destination: request.destination, Digest: digest, CopiedSource: copiedSource,
		RetriedOSOnly: outcome.retriedOSOnly, AlreadyCurrent: outcome.alreadyCurrent,
	}, nil
}

type patchOutcome struct {
	retriedOSOnly  bool
	alreadyCurrent bool
}

func runPatchPolicy(ctx context.Context, patcher Patcher, request *PatchRequest) (patchOutcome, error) {
	err := patcher.Patch(ctx, patchSpec(request, patchPackageModeFull))
	if err == nil {
		return patchOutcome{}, nil
	}
	if errors.Is(err, ErrNoPatchUpdates) {
		return patchOutcome{alreadyCurrent: true}, nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return patchOutcome{}, fmt.Errorf("patch image %s: %w", request.source, ctxErr)
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return patchOutcome{}, fmt.Errorf("patch image %s: %w", request.source, err)
	}
	if request.goVCSURL == "" && !isGoRebuildFailure(err.Error()) {
		return patchOutcome{}, fmt.Errorf("patch image %s: %w", request.source, err)
	}

	retryErr := patcher.Patch(ctx, patchSpec(request, patchPackageModeOSOnly))
	if retryErr == nil {
		return patchOutcome{retriedOSOnly: true}, nil
	}
	if errors.Is(retryErr, ErrNoPatchUpdates) {
		return patchOutcome{retriedOSOnly: true, alreadyCurrent: true}, nil
	}
	return patchOutcome{}, fmt.Errorf("retry patch with OS packages only: %w", retryErr)
}

func verifyStagingImage(
	ctx context.Context,
	commands CommandRunner,
	request *PatchRequest,
) (digest string, copiedSource bool, err error) {
	digestCheck, err := commands.Run(ctx, &Command{Name: "crane", Args: []string{"digest", request.destination}})
	if err != nil {
		return "", false, fmt.Errorf("check staging image digest: %w", err)
	}
	if digestCheck.ExitCode != 0 {
		if _, err := runRequiredCommand(ctx, commands, &Command{
			Name: "crane", Args: []string{"copy", "--platform", request.platform, request.source, request.destination},
		}); err != nil {
			return "", false, fmt.Errorf("copy clean source image: %w", err)
		}
		copiedSource = true
	}

	digestResult, err := runRequiredCommand(ctx, commands, &Command{Name: "crane", Args: []string{"digest", request.destination}})
	if err != nil {
		return "", false, fmt.Errorf("resolve staging image digest: %w", err)
	}
	digest = strings.TrimSpace(string(digestResult.Stdout))
	if !digestPattern.MatchString(digest) {
		return "", false, fmt.Errorf("%w: crane returned malformed digest %q", ErrInvalidPatchRequest, digest)
	}
	return digest, copiedSource, nil
}

func patchSpec(request *PatchRequest, mode patchPackageMode) *PatchSpec {
	spec := &PatchSpec{
		Source: request.source, Destination: request.destination, Report: request.report,
		PackageTypes: string(mode), BuildKitAddress: "buildx://copa-builder",
		Timeout: patchTimeout, Push: true,
	}
	if mode == patchPackageModeFull {
		spec.LibraryPatchLevel = "major"
		spec.ToolchainPatchLevel = "patch"
		spec.GoVCSURL = request.goVCSURL
	}
	return spec
}

func isGoRebuildFailure(message string) bool {
	patterns := []string{
		"go package upgrade operation failed", "no go.mod files detected", "no Go binaries detected",
		"no binaries were successfully rebuilt", "copa_discover_build.sh",
		`exec: "sh": executable file not found`, "repository does not contain ref", "Not a valid object name",
	}
	for _, pattern := range patterns {
		if strings.Contains(message, pattern) {
			return true
		}
	}
	return false
}
