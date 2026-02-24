package internal

import (
	"encoding/json"
	"errors"
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

func (d ImageDiscovery) Reference() string {
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
	Changed           bool   `json:"changed"`
}

// PublishedChart represents a chart that was published to OCI.
type PublishedChart struct {
	Name              string           `json:"name"`
	Version           string           `json:"version"`
	Registry          string           `json:"registry"`
	OCIRef            string           `json:"oci_ref"`
	SBOMPath          string           `json:"sbom_path"`
	VulnPredicatePath string           `json:"vuln_predicate_path"`
	Images            []PublishedImage `json:"images"`
}

// PublishedImage represents an image included in a published chart.
type PublishedImage struct {
	Original string `json:"original"`
	Patched  string `json:"patched"`
}

// AssembleResults reads a discovery manifest and patch results from matrix
// jobs, then creates wrapper charts. When publish is true and registry is set,
// publishes charts to OCI and generates SBOMs and vulnerability attestations.
// Only publishes charts where at least one underlying image changed.
func AssembleResults(manifestPath, resultsDir, reportsDir, outputDir, registry string, publish bool) error { //nolint:gocognit,gocyclo,cyclop,funlen // complex workflow
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

	var publishedCharts []PublishedChart

	// Create wrapper charts.
	for _, ch := range manifest.Charts {
		dep := Dependency{
			Name:       ch.Name,
			Version:    ch.Version,
			Repository: ch.Repository,
		}

		results := buildPatchResults(ch.Images, resultMap, reportsDir)

		// Check if any images changed
		hasChanges := false
		for _, imgDisc := range ch.Images {
			ref := Image(imgDisc).Reference()
			if r, ok := resultMap[ref]; ok && r.Changed {
				hasChanges = true
				break
			}
		}

		if !hasChanges {
			fmt.Printf("  Skipping %s: no images changed\n", ch.Name)
			continue
		}

		// Create wrapper chart
		version, err := CreateWrapperChart(dep, results, outputDir, registry)
		if err != nil {
			return fmt.Errorf("creating wrapper chart for %s: %w", ch.Name, err)
		}
		fmt.Printf("  Wrapper chart → %s/%s (%s)\n", outputDir, ch.Name, version)

		chartDir := filepath.Join(outputDir, ch.Name)
		ociRef := fmt.Sprintf("%s/charts/%s:%s", registry, ch.Name, version)

		// Publish to OCI if requested
		if publish && registry != "" {
			_, err := PublishChart(chartDir, registry)
			if err != nil {
				return fmt.Errorf("publishing chart %s: %w", ch.Name, err)
			}
		}

		// Generate SBOM
		sbomPath := filepath.Join(chartDir, "sbom.cdx.json")
		if err := GenerateChartSBOM(ch, results, version, sbomPath); err != nil {
			return fmt.Errorf("generating SBOM for %s: %w", ch.Name, err)
		}

		// Generate aggregated vulnerability predicate
		vulnPredicatePath := filepath.Join(chartDir, "vuln-predicate.json")
		if err := AggregateVulnPredicate(results, reportsDir, vulnPredicatePath); err != nil {
			return fmt.Errorf("generating vuln predicate for %s: %w", ch.Name, err)
		}

		// Record published chart
		pc := PublishedChart{
			Name:              ch.Name,
			Version:           version,
			Registry:          registry,
			OCIRef:            ociRef,
			SBOMPath:          sbomPath,
			VulnPredicatePath: vulnPredicatePath,
		}
		for _, pr := range results {
			// Include all successfully processed images (including mirrored ones that were skipped)
			if pr.Error == nil && pr.Patched.Reference() != "" {
				pc.Images = append(pc.Images, PublishedImage{
					Original: pr.Original.Reference(),
					Patched:  pr.Patched.Reference(),
				})
			}
		}
		publishedCharts = append(publishedCharts, pc)
	}

	// Write published-charts.json
	if len(publishedCharts) > 0 {
		data, err := json.MarshalIndent(publishedCharts, "", "  ")
		if err != nil {
			return fmt.Errorf("marshaling published charts: %w", err)
		}
		publishedPath := filepath.Join(outputDir, "published-charts.json")
		if err := os.WriteFile(publishedPath, data, 0o644); err != nil {
			return fmt.Errorf("writing published charts: %w", err)
		}
		fmt.Printf("\nPublished %d chart(s) → %s\n", len(publishedCharts), publishedPath)
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
			pr.SkipReason = SkipReasonNoPatchResult
			results = append(results, pr)
			continue
		}

		if r.Error != "" { //nolint:gocritic // prefer if-else for readability
			pr.Error = errors.New(r.Error) //nolint:err113 // wrapping error from JSON string
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
