package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// PatchOptions configures the patching pipeline.
type PatchOptions struct {
	// TargetRegistry is the registry to push patched images to (e.g. "ghcr.io/descope").
	// If empty, patched images are left in the local Docker daemon only.
	TargetRegistry string

	// BuildKitAddr is the BuildKit address for Copa (e.g. "docker-container://buildkitd").
	BuildKitAddr string

	// ReportDir is where Trivy JSON reports are written.
	ReportDir string
}

// PatchResult holds the outcome of patching a single image.
type PatchResult struct {
	Original  Image
	Patched   Image
	VulnCount int
	Skipped   bool
	Error     error
}

// PatchImage scans an image for OS vulnerabilities using Trivy,
// patches fixable ones with Copa, and optionally pushes the
// patched image to a target registry.
func PatchImage(ctx context.Context, img Image, opts PatchOptions) *PatchResult {
	result := &PatchResult{Original: img}
	ref := img.Reference()

	// 0. Pull the image so Trivy and Copa can access it locally.
	if err := dockerPull(ctx, ref); err != nil {
		result.Error = fmt.Errorf("pulling %s: %w", ref, err)
		return result
	}

	// 1. Scan with Trivy.
	reportPath := filepath.Join(opts.ReportDir, sanitize(ref)+".json")
	if err := trivyScan(ctx, ref, reportPath); err != nil {
		result.Error = fmt.Errorf("scanning %s: %w", ref, err)
		return result
	}

	vulns, err := countFixable(reportPath)
	if err != nil {
		result.Error = fmt.Errorf("reading report for %s: %w", ref, err)
		return result
	}
	result.VulnCount = vulns

	if vulns == 0 {
		result.Skipped = true
		result.Patched = img
		return result
	}

	// 2. Patch with Copa (requires BuildKit).
	tag := img.Tag
	if tag == "" {
		tag = "latest"
	}
	patchedTag := tag + "-patched"

	if err := copaPatch(ctx, ref, reportPath, patchedTag, opts.BuildKitAddr); err != nil {
		result.Error = fmt.Errorf("patching %s: %w", ref, err)
		return result
	}

	localPatched := img
	localPatched.Tag = patchedTag

	// 3. Optionally push to target registry.
	if opts.TargetRegistry != "" {
		target := Image{
			Registry:   opts.TargetRegistry,
			Repository: img.Repository,
			Tag:        patchedTag,
		}
		if err := dockerRetag(ctx, localPatched.Reference(), target.Reference()); err != nil {
			result.Error = fmt.Errorf("pushing %s: %w", target.Reference(), err)
			return result
		}
		result.Patched = target
	} else {
		result.Patched = localPatched
	}

	return result
}

// dockerPull pulls an image into the local Docker daemon.
func dockerPull(ctx context.Context, imageRef string) error {
	cmd := exec.CommandContext(ctx, "docker", "pull", imageRef)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker pull %s: %w", imageRef, err)
	}
	return nil
}

// trivyScan runs the trivy CLI to scan an image for OS vulnerabilities.
func trivyScan(ctx context.Context, imageRef, reportPath string) error {
	cmd := exec.CommandContext(ctx, "trivy", "image",
		"--vuln-type", "os",
		"--ignore-unfixed",
		"--format", "json",
		"--output", reportPath,
		imageRef,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("trivy image %s: %w", imageRef, err)
	}
	return nil
}

// copaPatch runs the copa CLI to patch an image via BuildKit.
func copaPatch(ctx context.Context, imageRef, reportPath, patchedTag, buildkitAddr string) error {
	args := []string{"patch",
		"--image", imageRef,
		"--report", reportPath,
		"--tag", patchedTag,
		"--timeout", "10m",
	}
	if buildkitAddr != "" {
		args = append(args, "--addr", buildkitAddr)
	}

	cmd := exec.CommandContext(ctx, "copa", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("copa patch %s: %w", imageRef, err)
	}
	return nil
}

// dockerRetag tags a local image and pushes it to a remote registry.
func dockerRetag(ctx context.Context, srcRef, dstRef string) error {
	tag := exec.CommandContext(ctx, "docker", "tag", srcRef, dstRef)
	tag.Stdout = os.Stdout
	tag.Stderr = os.Stderr
	if err := tag.Run(); err != nil {
		return fmt.Errorf("docker tag %s %s: %w", srcRef, dstRef, err)
	}

	push := exec.CommandContext(ctx, "docker", "push", dstRef)
	push.Stdout = os.Stdout
	push.Stderr = os.Stderr
	if err := push.Run(); err != nil {
		return fmt.Errorf("docker push %s: %w", dstRef, err)
	}
	return nil
}

// countFixable reads a Trivy JSON report and counts vulnerabilities with a fix available.
func countFixable(reportPath string) (int, error) {
	data, err := os.ReadFile(reportPath)
	if err != nil {
		return 0, err
	}
	var report trivyReport
	if err := json.Unmarshal(data, &report); err != nil {
		return 0, err
	}
	count := 0
	for _, r := range report.Results {
		for _, v := range r.Vulnerabilities {
			if v.FixedVersion != "" {
				count++
			}
		}
	}
	return count, nil
}

type trivyReport struct {
	Results []struct {
		Vulnerabilities []struct {
			FixedVersion string `json:"FixedVersion"`
		} `json:"Vulnerabilities"`
	} `json:"Results"`
}

func sanitize(ref string) string {
	r := strings.NewReplacer("/", "_", ":", "_")
	return r.Replace(ref)
}
