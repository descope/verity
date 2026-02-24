package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Trivy report structures (for aggregating vulnerability predicates).
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

// CycloneDX SBOM structures (simplified for chart use).
type cycloneDXSBOM struct {
	BOMFormat   string               `json:"bomFormat"`
	SpecVersion string               `json:"specVersion"`
	Version     int                  `json:"version"`
	Metadata    cycloneDXMetadata    `json:"metadata"`
	Components  []cycloneDXComponent `json:"components"`
}

type cycloneDXMetadata struct {
	Timestamp string             `json:"timestamp"`
	Component cycloneDXComponent `json:"component"`
}

type cycloneDXComponent struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Version string `json:"version"`
	PURL    string `json:"purl,omitempty"`
}

// GenerateChartSBOM creates a CycloneDX JSON SBOM for a chart.
// Lists the wrapper chart as the top component, the upstream chart as a
// dependency, and all patched images as components with pkg:oci PURLs.
func GenerateChartSBOM(chart ChartDiscovery, patchedImages []*PatchResult, wrapperVersion, outputPath string) error {
	// Top-level component: the wrapper chart itself
	topComponent := cycloneDXComponent{
		Type:    "application",
		Name:    chart.Name,
		Version: wrapperVersion,
	}

	// Components: upstream chart + all patched images
	var components []cycloneDXComponent

	// Add upstream chart as a dependency
	components = append(components, cycloneDXComponent{
		Type:    "application",
		Name:    chart.Name + " (upstream)",
		Version: chart.Version,
		PURL:    chartToPURL(chart),
	})

	// Add patched images (including mirrored images that were skipped with no fixable vulnerabilities)
	for _, pr := range patchedImages {
		// Only skip images that had errors or have no patched reference
		if pr.Error != nil || pr.Patched.Reference() == "" {
			continue
		}
		components = append(components, cycloneDXComponent{
			Type:    "container",
			Name:    pr.Patched.Repository,
			Version: pr.Patched.Tag,
			PURL:    imageToPURL(pr.Patched),
		})
	}

	sbom := cycloneDXSBOM{
		BOMFormat:   "CycloneDX",
		SpecVersion: "1.4",
		Version:     1,
		Metadata: cycloneDXMetadata{
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Component: topComponent,
		},
		Components: components,
	}

	data, err := json.MarshalIndent(sbom, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling SBOM: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("creating SBOM dir: %w", err)
	}

	if err := os.WriteFile(outputPath, data, 0o644); err != nil {
		return fmt.Errorf("writing SBOM: %w", err)
	}

	return nil
}

// chartToPURL converts a chart reference to a Package URL.
// Format: pkg:helm/[repo]/[name]@[version].
func chartToPURL(chart ChartDiscovery) string {
	// Simplify repository to just the hostname/org
	repo := strings.TrimPrefix(chart.Repository, "oci://")
	repo = strings.TrimPrefix(repo, "https://")
	repo = strings.TrimSuffix(repo, "/")

	return fmt.Sprintf("pkg:helm/%s/%s@%s", repo, chart.Name, chart.Version)
}

// imageToPURL converts an image to a Package URL.
// Format: pkg:oci/[repository]@[tag]?registry=[registry].
func imageToPURL(img Image) string {
	repo := strings.ReplaceAll(img.Repository, "/", "%2F")
	purl := fmt.Sprintf("pkg:oci/%s@%s", repo, img.Tag)
	if img.Registry != "" {
		purl += "?repository_url=" + img.Registry
	}
	return purl
}

// AggregateVulnPredicate aggregates Trivy reports from all underlying images
// into a single vulnerability predicate for the chart.
// The predicate format follows the cosign attest --type vuln schema.
func AggregateVulnPredicate(patchedImages []*PatchResult, reportsDir, outputPath string) error {
	type vulnEntry struct {
		VulnerabilityID  string `json:"VulnerabilityID"`
		PkgName          string `json:"PkgName"`
		Severity         string `json:"Severity"`
		InstalledVersion string `json:"InstalledVersion,omitempty"`
		FixedVersion     string `json:"FixedVersion,omitempty"`
		Title            string `json:"Title,omitempty"`
		Description      string `json:"Description,omitempty"`
		Image            string `json:"Image"` // Which image this vuln came from
	}

	type predicate struct {
		Invocation struct {
			URI string `json:"uri"`
		} `json:"invocation"`
		Scanner struct {
			URI     string `json:"uri"`
			Version string `json:"version"`
		} `json:"scanner"`
		Metadata struct {
			ScanStartedOn  string `json:"scanStartedOn"`
			ScanFinishedOn string `json:"scanFinishedOn"`
		} `json:"metadata"`
		Vulnerabilities []vulnEntry `json:"vulnerabilities"`
	}

	pred := predicate{}
	pred.Scanner.URI = "https://github.com/aquasecurity/trivy"
	pred.Scanner.Version = "aggregated"
	pred.Invocation.URI = "verity-chart-aggregate"

	now := time.Now().UTC().Format(time.RFC3339)
	pred.Metadata.ScanStartedOn = now
	pred.Metadata.ScanFinishedOn = now

	var allVulns []vulnEntry

	// Aggregate vulnerabilities from all image Trivy reports
	for _, pr := range patchedImages {
		if pr.Error != nil || pr.ReportPath == "" {
			continue
		}

		reportPath := pr.ReportPath
		if !filepath.IsAbs(reportPath) {
			reportPath = filepath.Join(reportsDir, filepath.Base(reportPath))
		}

		data, err := os.ReadFile(reportPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: cannot read report %s: %v\n", reportPath, err)
			continue
		}

		var report trivyReport
		if err := json.Unmarshal(data, &report); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: cannot parse report %s: %v\n", reportPath, err)
			continue
		}

		for _, res := range report.Results {
			for _, v := range res.Vulnerabilities {
				allVulns = append(allVulns, vulnEntry{
					VulnerabilityID:  v.VulnerabilityID,
					PkgName:          v.PkgName,
					Severity:         v.Severity,
					InstalledVersion: v.InstalledVersion,
					FixedVersion:     v.FixedVersion,
					Title:            v.Title,
					Description:      v.Description,
					Image:            pr.Patched.Reference(),
				})
			}
		}
	}

	pred.Vulnerabilities = allVulns

	data, err := json.MarshalIndent(pred, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling predicate: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("creating predicate dir: %w", err)
	}

	if err := os.WriteFile(outputPath, data, 0o644); err != nil {
		return fmt.Errorf("writing predicate: %w", err)
	}

	return nil
}
