package patchimage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/verity-org/verity/internal/ci/workflowops/retry"
)

var ErrEmptyDigest = errors.New("registry returned an empty digest")

type ManifestService struct {
	Runner retry.Runner
	Stdout io.Writer
	Stderr io.Writer
}

func (service ManifestService) Create(ctx context.Context, input ManifestPlanInput) (ManifestPlan, error) {
	plan, err := BuildManifestPlan(input)
	if err != nil {
		return ManifestPlan{}, err
	}
	for _, sourceTag := range plan.SourceTags {
		if _, err := service.runner().Run(ctx, &retry.Command{Name: "crane", Args: []string{"digest", sourceTag}}); err != nil {
			return ManifestPlan{}, fmt.Errorf("missing platform image %q: %w", sourceTag, err)
		}
	}
	args := make([]string, 0, 5+len(plan.SourceTags))
	args = append(args, "buildx", "imagetools", "create", "--tag", plan.ManifestTag)
	args = append(args, plan.SourceTags...)
	if _, err := service.runner().Run(ctx, &retry.Command{Name: "docker", Args: args}); err != nil {
		return ManifestPlan{}, fmt.Errorf("create multi-arch manifest: %w", err)
	}
	return plan, nil
}

type CopyManifestInput struct {
	ManifestTag    string
	TargetRegistry string
	ImageName      string
	SourceTag      string
	ManifestFile   string
}

type PublishedManifest struct {
	FinalRepository string
	FinalTag        string
	Digest          string
}

func (service ManifestService) Copy(ctx context.Context, input *CopyManifestInput) (PublishedManifest, error) {
	finalRepository := input.TargetRegistry + "/" + input.ImageName
	finalTag := finalRepository + ":" + input.SourceTag
	if _, err := service.runner().Run(ctx, &retry.Command{Name: "crane", Args: []string{"copy", input.ManifestTag, finalTag}}); err != nil {
		return PublishedManifest{}, fmt.Errorf("copy manifest to target: %w", err)
	}
	digest, err := service.digest(ctx, finalTag)
	if err != nil {
		return PublishedManifest{}, err
	}
	if err := os.WriteFile(input.ManifestFile, []byte(finalTag+"\n"), 0o600); err != nil {
		return PublishedManifest{}, fmt.Errorf("write final manifest file: %w", err)
	}
	return PublishedManifest{FinalRepository: finalRepository, FinalTag: finalTag, Digest: digest}, nil
}

func (service ManifestService) Resolve(ctx context.Context, reference string) (PublishedManifest, error) {
	digest, err := service.digest(ctx, reference)
	if err != nil {
		return PublishedManifest{}, err
	}
	return PublishedManifest{FinalTag: reference, Digest: digest}, nil
}

func (service ManifestService) Exists(ctx context.Context, reference string) bool {
	_, err := service.runner().Run(ctx, &retry.Command{Name: "crane", Args: []string{"digest", reference}})
	return err == nil
}

type SignManifestInput struct {
	Reference  string
	BundlePath string
	OutputPath string
}

type SignManifestResult struct {
	RekorURL string
}

func (service ManifestService) Sign(ctx context.Context, input SignManifestInput) (SignManifestResult, error) {
	for _, path := range []string{input.BundlePath, input.OutputPath} {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return SignManifestResult{}, fmt.Errorf("remove stale cosign file %q: %w", path, err)
		}
	}
	result, err := service.runner().Run(ctx, &retry.Command{
		Name: "cosign", Args: []string{"sign", "--yes", "--bundle", input.BundlePath, input.Reference},
	})
	combined := append(append([]byte(nil), result.Stdout...), result.Stderr...)
	if writeErr := os.WriteFile(input.OutputPath, combined, 0o600); writeErr != nil {
		return SignManifestResult{}, fmt.Errorf("write cosign output: %w", writeErr)
	}
	if _, writeErr := service.stdout().Write(result.Stdout); writeErr != nil {
		return SignManifestResult{}, fmt.Errorf("write cosign stdout: %w", writeErr)
	}
	if _, writeErr := service.stderr().Write(result.Stderr); writeErr != nil {
		return SignManifestResult{}, fmt.Errorf("write cosign stderr: %w", writeErr)
	}
	if err != nil {
		return SignManifestResult{}, fmt.Errorf("sign manifest: %w", err)
	}
	bundle, readErr := os.ReadFile(input.BundlePath)
	if readErr != nil && !os.IsNotExist(readErr) {
		return SignManifestResult{}, fmt.Errorf("read cosign bundle: %w", readErr)
	}
	return SignManifestResult{RekorURL: ExtractRekorURL(bundle, combined)}, nil
}

func (service ManifestService) digest(ctx context.Context, reference string) (string, error) {
	result, err := service.runner().Run(ctx, &retry.Command{Name: "crane", Args: []string{"digest", reference}})
	if err != nil {
		return "", fmt.Errorf("resolve digest for %q: %w", reference, err)
	}
	digest := strings.TrimSpace(string(result.Stdout))
	if digest == "" {
		return "", fmt.Errorf("%w: %s", ErrEmptyDigest, reference)
	}
	return digest, nil
}

func (service ManifestService) runner() retry.Runner {
	if service.Runner != nil {
		return service.Runner
	}
	return retry.ExecRunner{}
}

func (service ManifestService) stdout() io.Writer {
	if service.Stdout != nil {
		return service.Stdout
	}
	return io.Discard
}

func (service ManifestService) stderr() io.Writer {
	if service.Stderr != nil {
		return service.Stderr
	}
	return io.Discard
}
