package internal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var errEmptyDigest = errors.New("empty digest returned from registry")

const (
	statusPatched = "Patched"
	statusSkipped = "Skipped"
	statusFailed  = "Failed"
)

// PostProcessOptions holds configuration for post-processing Copa results.
type PostProcessOptions struct {
	CopaOutputPath   string
	ChartMapPath     string
	RegistryPrefix   string
	OutputDir        string
	SkipDigestLookup bool // for testing
}

// PostProcessResult represents the output of post-processing.
type PostProcessResult struct {
	MatrixPath   string
	ManifestPath string
	ResultsDir   string
	PatchedCount int
	SkippedCount int
	FailedCount  int
	ChartCount   int
	HasImages    bool
	HasCharts    bool
}

// PostProcessCopaResults reads Copa's output, queries registries, and generates
// the files needed by downstream jobs (matrix.json, manifest.json, result files).
func PostProcessCopaResults(opts PostProcessOptions) (*PostProcessResult, error) {
	// Parse Copa output
	copaOutput, err := ParseCopaOutput(opts.CopaOutputPath)
	if err != nil {
		return nil, fmt.Errorf("parsing copa output: %w", err)
	}

	// Parse chart-image mapping
	chartMap, err := ParseChartImageMap(opts.ChartMapPath)
	if err != nil {
		return nil, fmt.Errorf("parsing chart-image-map: %w", err)
	}

	// Create output directories
	if err := os.MkdirAll(opts.OutputDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating output dir: %w", err)
	}
	resultsDir := filepath.Join(opts.OutputDir, "results")
	if err := os.MkdirAll(resultsDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating results dir: %w", err)
	}

	result := &PostProcessResult{
		MatrixPath:   filepath.Join(opts.OutputDir, "matrix.json"),
		ManifestPath: filepath.Join(opts.OutputDir, "manifest.json"),
		ResultsDir:   resultsDir,
	}

	// Build lookup maps
	imageResultMap := buildImageResultMap(copaOutput.Results)

	// Generate matrix for attest job (only patched images)
	ctx := context.Background()
	matrix, err := generateMatrix(ctx, copaOutput.Results, opts.RegistryPrefix, opts.SkipDigestLookup)
	if err != nil {
		return nil, fmt.Errorf("generating matrix: %w", err)
	}
	result.HasImages = len(matrix.Include) > 0

	// Write matrix.json
	matrixData, err := json.Marshal(matrix)
	if err != nil {
		return nil, fmt.Errorf("marshaling matrix: %w", err)
	}
	if err := os.WriteFile(result.MatrixPath, matrixData, 0o644); err != nil {
		return nil, fmt.Errorf("writing matrix: %w", err)
	}

	// Generate manifest for assemble step (chart-grouped images)
	manifest := generateManifest(chartMap, imageResultMap)
	result.HasCharts = len(manifest.Charts) > 0
	result.ChartCount = len(manifest.Charts)

	// Write manifest.json
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshaling manifest: %w", err)
	}
	if err := os.WriteFile(result.ManifestPath, manifestData, 0o644); err != nil {
		return nil, fmt.Errorf("writing manifest: %w", err)
	}

	// Write per-image SinglePatchResult files for compatibility with assemble
	if err := writeResultFiles(copaOutput.Results, resultsDir); err != nil {
		return nil, fmt.Errorf("writing result files: %w", err)
	}

	// Count results by status
	for _, r := range copaOutput.Results {
		switch r.Status {
		case statusPatched:
			result.PatchedCount++
		case statusSkipped:
			result.SkippedCount++
		case statusFailed:
			result.FailedCount++
		}
	}

	return result, nil
}

// buildImageResultMap creates a lookup from normalized image reference to Copa result.
func buildImageResultMap(results []CopaOutputResult) map[string]*CopaOutputResult {
	m := make(map[string]*CopaOutputResult)
	for i := range results {
		r := &results[i]
		// Normalize source image for lookup
		normalized := NormalizeImageRef(r.SourceImage)
		m[normalized] = r
	}
	return m
}

// generateMatrix creates the GitHub Actions matrix for the attest job.
// Only includes successfully patched images (status="Patched").
func generateMatrix(ctx context.Context, results []CopaOutputResult, registryPrefix string, skipDigest bool) (*MatrixOutput, error) {
	matrix := &MatrixOutput{}

	for _, r := range results {
		if r.Status != statusPatched {
			continue
		}

		// Get digest for the patched image
		patchedRef := r.PatchedImage
		if !skipDigest {
			digest, err := getImageDigest(ctx, patchedRef)
			if err != nil {
				// Be resilient: skip images whose digest cannot be retrieved, but continue processing others
				fmt.Fprintf(os.Stderr, "Warning: skipping image %s because digest lookup failed: %v\n", patchedRef, err)
				continue
			}
			// Use digest for signing/attestation
			registry, repository, _ := ParseImageRef(patchedRef)
			if registry != "" {
				patchedRef = registry + "/" + repository + "@" + digest
			} else {
				patchedRef = repository + "@" + digest
			}
		}

		// Create matrix entry
		entry := MatrixEntry{
			ImageRef:  patchedRef,
			ImageName: sanitizeImageName(r.SourceImage),
		}
		matrix.Include = append(matrix.Include, entry)
	}

	return matrix, nil
}

// generateManifest creates the DiscoveryManifest structure for the assemble step.
// Groups images by chart according to chart-image-map.yaml.
func generateManifest(chartMap *ChartImageMap, resultMap map[string]*CopaOutputResult) *DiscoveryManifest {
	manifest := &DiscoveryManifest{}

	for _, chartEntry := range chartMap.Charts {
		cd := ChartDiscovery{
			Name:       chartEntry.Name,
			Version:    chartEntry.Version,
			Repository: chartEntry.Repository,
		}

		// Add images for this chart
		for _, imgRef := range chartEntry.Images {
			normalized := NormalizeImageRef(imgRef)
			registry, repository, tag := ParseImageRef(normalized)

			imgDisc := ImageDiscovery{
				Registry:   registry,
				Repository: repository,
				Tag:        tag,
				Path:       "", // Not needed for Copa-based workflow
			}
			cd.Images = append(cd.Images, imgDisc)

			// Add to flat images list
			manifest.Images = append(manifest.Images, imgDisc)
		}

		manifest.Charts = append(manifest.Charts, cd)
	}

	return manifest
}

// writeResultFiles writes SinglePatchResult JSON files for each Copa result.
// These files are read by the assemble step to create wrapper charts.
func writeResultFiles(results []CopaOutputResult, resultsDir string) error {
	for _, r := range results {
		sourceRef := r.SourceImage
		patchedRef := r.PatchedImage

		// Parse patched image
		patchedRegistry, patchedRepository, patchedTag := ParseImageRef(patchedRef)

		// Create SinglePatchResult
		spr := SinglePatchResult{
			ImageRef: sourceRef,
		}

		switch r.Status {
		case statusPatched:
			spr.PatchedRegistry = patchedRegistry
			spr.PatchedRepository = patchedRepository
			spr.PatchedTag = patchedTag
			spr.Changed = true
			// VulnCount would come from scanning, not available here
			spr.VulnCount = 0

		case statusSkipped:
			spr.Skipped = true
			spr.SkipReason = r.Details
			// If skipped but still has a patched ref, record it
			if patchedRef != "" && patchedRef != sourceRef {
				spr.PatchedRegistry = patchedRegistry
				spr.PatchedRepository = patchedRepository
				spr.PatchedTag = patchedTag
			}

		case statusFailed:
			spr.Error = r.Details
		}

		// Write result file
		filename := sanitizeImageName(sourceRef) + ".json"
		resultPath := filepath.Join(resultsDir, filename)

		data, err := json.MarshalIndent(spr, "", "  ")
		if err != nil {
			return fmt.Errorf("marshaling result for %s: %w", sourceRef, err)
		}

		if err := os.WriteFile(resultPath, data, 0o644); err != nil {
			return fmt.Errorf("writing result file for %s: %w", sourceRef, err)
		}

		fmt.Printf("  Wrote result: %s (%s)\n", filename, r.Status)
	}

	return nil
}

// getImageDigest queries the registry for an image's digest using crane.
func getImageDigest(ctx context.Context, ref string) (string, error) {
	cmd := exec.CommandContext(ctx, "crane", "digest", ref)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("crane digest failed for %s: %w\nOutput: %s", ref, err, string(output))
	}

	digest := strings.TrimSpace(string(output))
	if digest == "" {
		return "", fmt.Errorf("%w for %s", errEmptyDigest, ref)
	}

	return digest, nil
}

// sanitizeImageName converts an image reference to a safe filename/artifact name.
func sanitizeImageName(ref string) string {
	// Remove protocol if present
	ref = strings.TrimPrefix(ref, "https://")
	ref = strings.TrimPrefix(ref, "http://")

	// Replace special characters with underscores
	replacer := strings.NewReplacer(
		"/", "_",
		":", "_",
		"@", "_",
		".", "_",
	)

	return replacer.Replace(ref)
}
