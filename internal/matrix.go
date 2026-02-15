package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DiscoveryManifest holds all discovered images.
// Charts groups images by chart dependency (used by the assemble step).
// Images is the unified flat list from values.yaml (used for matrix generation).
// Written by the discover step, read by the assemble step.
type DiscoveryManifest struct {
	Charts []ChartDiscovery `json:"charts"`
	Images []ImageDiscovery `json:"images"`
}

// ChartDiscovery groups images found in a single Helm chart dependency.
type ChartDiscovery struct {
	Name       string           `json:"name"`
	Version    string           `json:"version"`
	Repository string           `json:"repository"`
	Images     []ImageDiscovery `json:"images"`
}

// ImageDiscovery is a single discovered image with its values path.
type ImageDiscovery struct {
	Registry   string `json:"registry"`
	Repository string `json:"repository"`
	Tag        string `json:"tag"`
	Path       string `json:"path"`
}

func (d ImageDiscovery) reference() string {
	img := Image{Registry: d.Registry, Repository: d.Repository, Tag: d.Tag}
	return img.Reference()
}

// MatrixEntry represents one job in a GitHub Actions matrix.
type MatrixEntry struct {
	ImageRef  string `json:"image_ref"`
	ImageName string `json:"image_name"` // sanitized ref, used for artifact naming
}

// MatrixOutput is the GitHub Actions matrix JSON.
type MatrixOutput struct {
	Include []MatrixEntry `json:"include"`
}

// SinglePatchResult is the JSON written by each matrix job after patching.
type SinglePatchResult struct {
	ImageRef          string `json:"image_ref"`
	PatchedRegistry   string `json:"patched_registry,omitempty"`
	PatchedRepository string `json:"patched_repository,omitempty"`
	PatchedTag        string `json:"patched_tag,omitempty"`
	VulnCount         int    `json:"vuln_count"`
	Skipped           bool   `json:"skipped"`
	SkipReason        string `json:"skip_reason,omitempty"`
	Error             string `json:"error,omitempty"`
}

// DiscoverImages scans Chart.yaml dependencies and the images file,
// returning a manifest of all images and a deduplicated matrix for
// GitHub Actions.
//
// Chart-discovered images are merged into the images file (values.yaml)
// so that it becomes the single source of truth for all images. The
// manifest retains chart→images grouping for the assemble step, while
// manifest.Images holds the unified flat list from the images file.
func DiscoverImages(chartFile, imagesFile, tmpDir string) (*DiscoveryManifest, error) {
	manifest := &DiscoveryManifest{}

	chart, err := ParseChartFile(chartFile)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", chartFile, err)
	}

	var chartImages []Image

	for _, dep := range chart.Dependencies {
		fmt.Printf("Discovering %s@%s\n", dep.Name, dep.Version)

		chartPath, err := DownloadChart(dep, tmpDir)
		if err != nil {
			return nil, fmt.Errorf("downloading %s: %w", dep.Name, err)
		}

		images, err := ScanForImages(chartPath)
		if err != nil {
			return nil, fmt.Errorf("scanning %s: %w", dep.Name, err)
		}

		cd := ChartDiscovery{
			Name:       dep.Name,
			Version:    dep.Version,
			Repository: dep.Repository,
		}
		for _, img := range images {
			cd.Images = append(cd.Images, ImageDiscovery(img))
		}
		fmt.Printf("  Found %d images\n", len(images))
		manifest.Charts = append(manifest.Charts, cd)

		chartImages = append(chartImages, images...)
	}

	// Merge chart-discovered images into the images file so it contains
	// all images (chart-discovered + manually maintained standalone).
	if imagesFile != "" {
		if err := MergeChartImages(imagesFile, chartImages); err != nil {
			return nil, fmt.Errorf("merging chart images into %s: %w", imagesFile, err)
		}

		// Read the unified image list back from the updated file.
		allImages, err := ParseImagesFile(imagesFile)
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", imagesFile, err)
		}
		for _, img := range allImages {
			manifest.Images = append(manifest.Images, ImageDiscovery(img))
		}
		fmt.Printf("Total images: %d\n", len(allImages))
	} else {
		// No images file provided; fall back to chart-discovered images directly.
		for _, img := range chartImages {
			manifest.Images = append(manifest.Images, ImageDiscovery(img))
		}
		fmt.Printf("Total images: %d\n", len(chartImages))
	}

	return manifest, nil
}

// GenerateMatrix creates a deduplicated GitHub Actions matrix from a manifest.
// Uses the unified Images list so every image is patched exactly once.
func GenerateMatrix(manifest *DiscoveryManifest) *MatrixOutput {
	seen := make(map[string]bool)
	matrix := &MatrixOutput{}

	for _, img := range manifest.Images {
		ref := img.reference()
		if seen[ref] {
			continue
		}
		seen[ref] = true
		matrix.Include = append(matrix.Include, MatrixEntry{
			ImageRef:  ref,
			ImageName: sanitize(ref),
		})
	}

	return matrix
}

// WriteDiscoveryOutput writes the manifest and matrix JSON files.
// The matrix JSON is compact (single line) so it can be set as a
// GitHub Actions output directly.
func WriteDiscoveryOutput(manifest *DiscoveryManifest, matrix *MatrixOutput, outputDir string) error {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("creating output dir: %w", err)
	}

	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(outputDir, "manifest.json"), manifestData, 0o644); err != nil {
		return fmt.Errorf("writing manifest: %w", err)
	}

	// Compact JSON for GITHUB_OUTPUT (must be single line).
	matrixData, err := json.Marshal(matrix)
	if err != nil {
		return fmt.Errorf("marshaling matrix: %w", err)
	}
	if err := os.WriteFile(filepath.Join(outputDir, "matrix.json"), matrixData, 0o644); err != nil {
		return fmt.Errorf("writing matrix: %w", err)
	}

	return nil
}

// PatchSingleImage patches one image and writes the result JSON and
// Trivy report to the given directories. Designed to run in a matrix job.
func PatchSingleImage(ctx context.Context, imageRef string, opts PatchOptions, resultDir string) error {
	img := parseRef(imageRef)

	if err := os.MkdirAll(resultDir, 0o755); err != nil {
		return fmt.Errorf("creating result dir: %w", err)
	}
	if err := os.MkdirAll(opts.ReportDir, 0o755); err != nil {
		return fmt.Errorf("creating report dir: %w", err)
	}

	result := PatchImage(ctx, img, opts)

	entry := SinglePatchResult{
		ImageRef:   imageRef,
		VulnCount:  result.VulnCount,
		Skipped:    result.Skipped,
		SkipReason: result.SkipReason,
	}
	if result.Error != nil {
		entry.Error = result.Error.Error()
	}
	if !result.Skipped && result.Error == nil {
		entry.PatchedRegistry = result.Patched.Registry
		entry.PatchedRepository = result.Patched.Repository
		entry.PatchedTag = result.Patched.Tag
	}
	// For skipped images that have a genuinely different patched ref
	// (e.g. already patched in registry), record it. Don't record when
	// the patched ref equals the original upstream ref.
	if result.Skipped && result.Patched.Repository != "" &&
		result.Patched.Reference() != result.Original.Reference() {
		entry.PatchedRegistry = result.Patched.Registry
		entry.PatchedRepository = result.Patched.Repository
		entry.PatchedTag = result.Patched.Tag
	}

	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling result: %w", err)
	}

	resultPath := filepath.Join(resultDir, sanitize(imageRef)+".json")
	if err := os.WriteFile(resultPath, data, 0o644); err != nil {
		return fmt.Errorf("writing result: %w", err)
	}

	// Fail if the patch operation had an error (push failure, Copa error, etc.)
	// This ensures matrix jobs fail loudly instead of silently writing error
	// to JSON and causing data loss in the assemble step.
	if result.Error != nil {
		return fmt.Errorf("patch failed for %s: %w", imageRef, result.Error)
	}

	return nil
}

// AssembleResults reads a discovery manifest and patch results from matrix
// jobs, then creates wrapper charts. Vulnerability reports are attached as
// in-toto attestations on each image (handled by the CI workflow), so they
// are not bundled into the chart packages.
func AssembleResults(manifestPath, resultsDir, reportsDir, outputDir, registry string) error {
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("reading manifest: %w", err)
	}
	var manifest DiscoveryManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return fmt.Errorf("parsing manifest: %w", err)
	}

	// Load all patch results keyed by image ref.
	resultMap, err := loadResults(resultsDir)
	if err != nil {
		return err
	}

	// Create wrapper charts.
	for _, ch := range manifest.Charts {
		dep := Dependency{
			Name:       ch.Name,
			Version:    ch.Version,
			Repository: ch.Repository,
		}

		results := buildPatchResults(ch.Images, resultMap, reportsDir)

		version, err := CreateWrapperChart(dep, results, outputDir, registry)
		if err != nil {
			return fmt.Errorf("creating wrapper chart for %s: %w", ch.Name, err)
		}
		fmt.Printf("  Wrapper chart → %s/%s (%s)\n", outputDir, ch.Name, version)
	}

	return nil
}

// loadResults reads all SinglePatchResult JSON files from a directory,
// returning a map keyed by image reference.
func loadResults(dir string) (map[string]*SinglePatchResult, error) {
	m := make(map[string]*SinglePatchResult)

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return m, nil
		}
		return nil, fmt.Errorf("reading results dir: %w", err)
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		filename := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(filename)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: cannot read result file %s: %v\n", e.Name(), err)
			continue
		}
		var r SinglePatchResult
		if err := json.Unmarshal(data, &r); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: cannot parse result file %s: %v\n", e.Name(), err)
			continue
		}
		if r.ImageRef == "" {
			fmt.Fprintf(os.Stderr, "Warning: result file %s has empty ImageRef, skipping\n", e.Name())
			continue
		}
		m[r.ImageRef] = &r
	}

	return m, nil
}

// buildPatchResults converts discovered images + matrix results into
// PatchResult objects that CreateWrapperChart expects.
func buildPatchResults(images []ImageDiscovery, resultMap map[string]*SinglePatchResult, reportsDir string) []*PatchResult {
	var results []*PatchResult

	for _, imgDisc := range images {
		img := Image(imgDisc)
		ref := img.Reference()

		pr := &PatchResult{Original: img}

		r, ok := resultMap[ref]
		if !ok || r == nil {
			// No patch result produced (matrix job may have failed).
			pr.Skipped = true
			pr.SkipReason = "no patch result for image"
			results = append(results, pr)
			continue
		}

		if r.Error != "" {
			pr.Error = fmt.Errorf("%s", r.Error)
		} else if r.Skipped {
			pr.Skipped = true
			pr.SkipReason = r.SkipReason
			if r.PatchedRepository != "" {
				pr.Patched = Image{
					Registry:   r.PatchedRegistry,
					Repository: r.PatchedRepository,
					Tag:        r.PatchedTag,
				}
			}
		} else {
			pr.VulnCount = r.VulnCount
			pr.Patched = Image{
				Registry:   r.PatchedRegistry,
				Repository: r.PatchedRepository,
				Tag:        r.PatchedTag,
			}
		}

		// Look for trivy report by sanitized original ref.
		reportPath := filepath.Join(reportsDir, sanitize(ref)+".json")
		if _, err := os.Stat(reportPath); err == nil {
			pr.ReportPath = reportPath
			pr.UpstreamReportPath = reportPath
		}

		results = append(results, pr)
	}

	return results
}
