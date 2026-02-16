package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/crane"
	v1 "github.com/google/go-containerregistry/pkg/v1"
)

// PatchOptions configures the patching pipeline.
type PatchOptions struct {
	// TargetRegistry is the registry to push patched images to (e.g. "ghcr.io/verity-org").
	// If empty, patched images are left in the local Docker daemon only.
	TargetRegistry string

	// BuildKitAddr is the BuildKit address for Copa (e.g. "docker-container://buildkitd").
	BuildKitAddr string

	// ReportDir is where Trivy JSON reports are written.
	ReportDir string

	// WorkDir is a temporary directory for storing OCI image layouts.
	WorkDir string
}

// PatchResult holds the outcome of patching a single image.
type PatchResult struct {
	Original           Image
	Patched            Image
	VulnCount          int
	Skipped            bool
	SkipReason         string // Human-readable reason when Skipped is true
	Error              error
	ReportPath         string // Path to Trivy JSON report (may be patched image scan)
	UpstreamReportPath string // Path to Trivy JSON report of the original upstream image
	OverriddenFrom     string // Original tag before override (empty if not overridden)
}

// PatchImage scans an image for OS vulnerabilities using Trivy,
// patches fixable ones with Copa, and optionally pushes the
// patched image to a target registry.
//
// When a target registry is set, it first checks whether a patched
// image already exists there. If so, it scans the patched image
// instead of the upstream — skipping entirely when no new fixable
// vulns are found, or re-patching from upstream when they are.
func PatchImage(ctx context.Context, img Image, opts PatchOptions) *PatchResult {
	result := &PatchResult{Original: img}

	tag := img.Tag
	if tag == "" {
		tag = "latest"
	}
	patchedTag := tag + "-patched"

	// Check if a patched image already exists in the target registry.
	if opts.TargetRegistry != "" {
		patchedRef := Image{
			Registry:   opts.TargetRegistry,
			Repository: img.Repository,
			Tag:        patchedTag,
		}
		if imageExists(ctx, patchedRef.Reference()) {
			fmt.Printf("    Found existing patched image %s, checking for new vulns ...\n", patchedRef.Reference())

			// Scan the existing patched image for new fixable vulns.
			ociDir := filepath.Join(opts.WorkDir, "oci", sanitize(patchedRef.Reference()))
			if err := pullAndSaveOCI(ctx, patchedRef.Reference(), ociDir); err != nil {
				result.Error = fmt.Errorf("pulling patched image %s: %w", patchedRef.Reference(), err)
				return result
			}

			reportPath := filepath.Join(opts.ReportDir, sanitize(patchedRef.Reference())+".json")
			result.ReportPath = reportPath
			if err := trivyScan(ctx, ociDir, reportPath); err != nil {
				result.Error = fmt.Errorf("scanning patched image %s: %w", patchedRef.Reference(), err)
				return result
			}

			vulns, err := countFixable(reportPath)
			if err != nil {
				result.Error = fmt.Errorf("reading report for %s: %w", patchedRef.Reference(), err)
				return result
			}

			// Also scan the original upstream image so we have "before" data.
			upstreamRef := img.Reference()
			upstreamOciDir := filepath.Join(opts.WorkDir, "oci", sanitize(upstreamRef))
			upstreamReportPath := filepath.Join(opts.ReportDir, sanitize(upstreamRef)+".json")
			if err := pullAndSaveOCI(ctx, upstreamRef, upstreamOciDir); err != nil {
				fmt.Printf("    WARN: could not pull upstream %s for report: %v\n", upstreamRef, err)
			} else if err := trivyScan(ctx, upstreamOciDir, upstreamReportPath); err != nil {
				fmt.Printf("    WARN: could not scan upstream %s for report: %v\n", upstreamRef, err)
			} else {
				result.UpstreamReportPath = upstreamReportPath
			}

			if vulns == 0 {
				result.Skipped = true
				result.SkipReason = "patched image up to date"
				result.Patched = patchedRef
				return result
			}

			fmt.Printf("    Patched image has %d new fixable vuln(s), re-patching from upstream ...\n", vulns)
			// Fall through to re-patch from upstream.
		}
	}

	// Normal flow: pull upstream, scan, patch, push.
	ref := img.Reference()
	ociDir := filepath.Join(opts.WorkDir, "oci", sanitize(ref))
	if err := pullAndSaveOCI(ctx, ref, ociDir); err != nil {
		result.Error = fmt.Errorf("pulling %s: %w", ref, err)
		return result
	}

	reportPath := filepath.Join(opts.ReportDir, sanitize(ref)+".json")
	result.ReportPath = reportPath
	result.UpstreamReportPath = reportPath
	if err := trivyScan(ctx, ociDir, reportPath); err != nil {
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
		result.SkipReason = "no fixable vulnerabilities"

		// Mirror the image to the target registry even when no patching is
		// needed, so consumers always see the latest version available and
		// have a clear upgrade path.
		if opts.TargetRegistry != "" {
			target := Image{
				Registry:   opts.TargetRegistry,
				Repository: img.Repository,
				Tag:        patchedTag,
			}
			if err := mirrorImage(ctx, ref, target.Reference()); err != nil {
				result.Error = fmt.Errorf("mirroring %s to %s: %w", ref, target.Reference(), err)
				return result
			}
			result.Patched = target
		} else {
			result.Patched = img
		}
		return result
	}

	// Patch with Copa (requires BuildKit).
	if err := copaPatch(ctx, ref, reportPath, patchedTag, opts.BuildKitAddr); err != nil {
		result.Error = fmt.Errorf("patching %s: %w", ref, err)
		return result
	}

	localPatched := img
	localPatched.Tag = patchedTag

	// Optionally push to target registry.
	if opts.TargetRegistry != "" {
		target := Image{
			Registry:   opts.TargetRegistry,
			Repository: img.Repository,
			Tag:        patchedTag,
		}
		if err := pushLocal(ctx, localPatched.Reference(), target.Reference()); err != nil {
			result.Error = fmt.Errorf("pushing %s: %w", target.Reference(), err)
			return result
		}
		result.Patched = target
	} else {
		result.Patched = localPatched
	}

	return result
}

// imageExists checks whether an image reference exists in a remote registry
// using a HEAD request (crane.Head). Returns false on any error.
func imageExists(ctx context.Context, ref string) bool {
	opts := []crane.Option{
		crane.WithAuthFromKeychain(authn.DefaultKeychain),
		crane.WithContext(ctx),
	}
	_, err := crane.Head(ref, opts...)
	return err == nil
}

// pullAndSaveOCI pulls an image from a registry using go-containerregistry
// and saves it as an OCI layout directory for offline scanning.
func pullAndSaveOCI(ctx context.Context, imageRef, ociDir string) error {
	opts := []crane.Option{
		crane.WithAuthFromKeychain(authn.DefaultKeychain),
		crane.WithContext(ctx),
		crane.WithPlatform(&v1.Platform{OS: "linux", Architecture: "amd64"}),
	}

	fmt.Printf("    Pulling %s ...\n", imageRef)
	img, err := crane.Pull(imageRef, opts...)
	if err != nil {
		return fmt.Errorf("pulling %s: %w", imageRef, err)
	}

	if err := os.MkdirAll(filepath.Dir(ociDir), 0o755); err != nil {
		return err
	}
	if err := crane.SaveOCI(img, ociDir); err != nil {
		return fmt.Errorf("saving OCI layout for %s: %w", imageRef, err)
	}
	return nil
}

// trivyScan runs the trivy CLI to scan an OCI image layout for OS vulnerabilities.
func trivyScan(ctx context.Context, ociDir, reportPath string) error {
	cmd := exec.CommandContext(ctx, "trivy", "image",
		"--input", ociDir,
		"--vuln-type", "os",
		"--ignore-unfixed",
		"--format", "json",
		"--output", reportPath,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("trivy scan %s: %w", ociDir, err)
	}
	return nil
}

// copaPatch runs the copa CLI to patch an image via BuildKit.
func copaPatch(ctx context.Context, imageRef, reportPath, patchedTag, buildkitAddr string) error {
	args := []string{
		"patch",
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

// mirrorImage copies an image between registries using crane.Copy.
// Used to publish images that need no patching to the target registry.
func mirrorImage(ctx context.Context, srcRef, dstRef string) error {
	opts := []crane.Option{
		crane.WithAuthFromKeychain(authn.DefaultKeychain),
		crane.WithContext(ctx),
	}
	fmt.Printf("    Mirroring %s → %s ...\n", srcRef, dstRef)
	return crane.Copy(srcRef, dstRef, opts...)
}

// pushLocal saves a local Docker image to a tarball and pushes it to a
// remote registry using crane. This avoids "manifest invalid" errors that
// docker push can produce on some registries (e.g. Quay.io) when the
// image was built by Copa/BuildKit with Docker media types.
func pushLocal(ctx context.Context, srcRef, dstRef string) error {
	tmp, err := os.CreateTemp("", "verity-image-*.tar")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	_ = tmp.Close()
	defer func() { _ = os.Remove(tmp.Name()) }()

	save := exec.CommandContext(ctx, "docker", "save", "-o", tmp.Name(), srcRef)
	save.Stdout = os.Stdout
	save.Stderr = os.Stderr
	if err := save.Run(); err != nil {
		return fmt.Errorf("docker save %s: %w", srcRef, err)
	}

	img, err := crane.Load(tmp.Name())
	if err != nil {
		return fmt.Errorf("loading image %s: %w", srcRef, err)
	}

	fmt.Printf("    Pushing %s ...\n", dstRef)
	return crane.Push(img, dstRef,
		crane.WithAuthFromKeychain(authn.DefaultKeychain),
		crane.WithContext(ctx),
	)
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
		Vulnerabilities []trivyVulnerability `json:"Vulnerabilities"`
	} `json:"Results"`
}

type trivyVulnerability struct {
	VulnerabilityID  string `json:"VulnerabilityID"`
	PkgName          string `json:"PkgName"`
	Severity         string `json:"Severity"`
	InstalledVersion string `json:"InstalledVersion"`
	FixedVersion     string `json:"FixedVersion"`
	Title            string `json:"Title"`
	Description      string `json:"Description"`
}

func sanitize(ref string) string {
	r := strings.NewReplacer("/", "_", ":", "_")
	return r.Replace(ref)
}
