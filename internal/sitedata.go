package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// SiteData is the top-level structure for the catalog JSON consumed by the Astro site.
type SiteData struct {
	GeneratedAt string      `json:"generatedAt"`
	Registry    string      `json:"registry"`
	Summary     SiteSummary `json:"summary"`
	Charts      []SiteChart `json:"charts"`
	Images      []SiteImage `json:"images"`
}

// SiteChart describes a Helm chart and the container images it contains.
type SiteChart struct {
	Name            string      `json:"name"`
	Version         string      `json:"version"`
	UpstreamVersion string      `json:"upstreamVersion"`
	Description     string      `json:"description"`
	Repository      string      `json:"repository"`
	HelmInstall     string      `json:"helmInstall"`
	Images          []SiteImage `json:"images"`
}

// SiteSummary aggregates stats across all images.
type SiteSummary struct {
	TotalCharts  int `json:"totalCharts"`
	TotalImages  int `json:"totalImages"`
	TotalVulns   int `json:"totalVulns"`
	FixableVulns int `json:"fixableVulns"`
}

// SiteImage describes a single container image with its vulnerability data.
type SiteImage struct {
	ID              string      `json:"id"`
	OriginalRef     string      `json:"originalRef"`
	PatchedRef      string      `json:"patchedRef"`
	ValuesPath      string      `json:"valuesPath"`
	OS              string      `json:"os"`
	OverriddenFrom  string      `json:"overriddenFrom,omitempty"`
	VulnSummary     VulnSummary `json:"vulnSummary"`
	Vulnerabilities []SiteVuln  `json:"vulnerabilities"`
}

// VulnSummary counts vulnerabilities by severity.
type VulnSummary struct {
	Total          int            `json:"total"`
	Fixable        int            `json:"fixable"`
	SeverityCounts map[string]int `json:"severityCounts"`
}

// SiteVuln represents a single vulnerability entry.
type SiteVuln struct {
	ID               string `json:"id"`
	PkgName          string `json:"pkgName"`
	InstalledVersion string `json:"installedVersion"`
	FixedVersion     string `json:"fixedVersion"`
	Severity         string `json:"severity"`
	Title            string `json:"title"`
}

// trivyReportFull is an expanded version of trivyReport that captures severity,
// package info, and OS metadata from Trivy JSON reports.
type trivyReportFull struct {
	Metadata struct {
		OS struct {
			Family string `json:"Family"`
			Name   string `json:"Name"`
		} `json:"OS"`
	} `json:"Metadata"`
	Results []trivyResultFull `json:"Results"`
}

type trivyResultFull struct {
	Vulnerabilities []trivyVulnFull `json:"Vulnerabilities"`
}

type trivyVulnFull struct {
	VulnerabilityID  string `json:"VulnerabilityID"`
	PkgName          string `json:"PkgName"`
	InstalledVersion string `json:"InstalledVersion"`
	FixedVersion     string `json:"FixedVersion"`
	Severity         string `json:"Severity"`
	Title            string `json:"Title"`
}

// SaveOverrides writes a mapping of sanitized image ref → original tag
// to an overrides.json file in the given directory.
func SaveOverrides(results []*PatchResult, dir string) error {
	overrides := make(map[string]string)
	for _, r := range results {
		if r.OverriddenFrom != "" {
			key := sanitize(r.Original.Reference())
			overrides[key] = r.OverriddenFrom
		}
	}
	if len(overrides) == 0 {
		return nil
	}
	data, err := json.MarshalIndent(overrides, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "overrides.json"), data, 0o644)
}

// SaveImagePaths writes a mapping of sanitized image ref → Helm values path
// so that site data generation can populate valuesPath even for unpatched images.
func SaveImagePaths(results []*PatchResult, dir string) error {
	paths := make(map[string]string)
	for _, r := range results {
		if r.Original.Path != "" {
			key := sanitize(r.Original.Reference())
			paths[key] = r.Original.Path
		}
	}
	if len(paths) == 0 {
		return nil
	}
	data, err := json.MarshalIndent(paths, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "paths.json"), data, 0o644)
}

// ImageEntry represents a single image entry from the sign-and-attest.sh output.
type ImageEntry struct {
	Original string `json:"original"`
	Patched  string `json:"patched"`
	Report   string `json:"report"`
}

// GenerateSiteDataFromJSON reads images.json (from sign-and-attest.sh) to produce a catalog.json.
// This is the bulk config mode path that replaces values.yaml-based catalog generation.
func GenerateSiteDataFromJSON(imagesJSON, reportsDir, registry, outputPath string) error {
	// Read images.json
	data, err := os.ReadFile(imagesJSON)
	if err != nil {
		return fmt.Errorf("reading %s: %w", imagesJSON, err)
	}

	var entries []ImageEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return fmt.Errorf("parsing %s: %w", imagesJSON, err)
	}

	siteData := SiteData{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Registry:    registry,
		Charts:      []SiteChart{},
	}

	var allImages []SiteImage

	for _, entry := range entries {
		// Parse the original and patched refs
		originalRef := entry.Original
		patchedRef := entry.Patched
		sanitizedRef := sanitize(originalRef)

		// Try to find and parse the report
		var si SiteImage
		if entry.Report != "" && fileExists(entry.Report) {
			if report, err := parseTrivyReportFull(entry.Report); err == nil {
				si = buildSiteImage(sanitizedRef, originalRef, patchedRef, "", report)
			} else {
				fmt.Fprintf(os.Stderr, "Warning: failed to parse Trivy report %s: %v\n", entry.Report, err)
			}
		}

		// Fallback to empty site image if no report
		if si.ID == "" {
			si = SiteImage{
				ID:          sanitizedRef,
				OriginalRef: originalRef,
				PatchedRef:  patchedRef,
				VulnSummary: VulnSummary{
					SeverityCounts: make(map[string]int),
				},
				Vulnerabilities: []SiteVuln{},
			}
		}

		allImages = append(allImages, si)
	}

	siteData.Images = allImages
	siteData.Summary = computeSummary(allImages)

	// Marshal and write
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	out, err := json.MarshalIndent(siteData, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling site data: %w", err)
	}
	return os.WriteFile(outputPath, out, 0o644)
}

// fileExists checks if a file exists at the given path.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// GenerateSiteData reads the images file to produce a catalog.json for the Astro static site.
//
// The images file (values.yaml) is the single source of truth for all images.
// reportsDir optionally provides local Trivy reports (available during CI from the patch step artifacts).
func parseTrivyReportFull(path string) (*trivyReportFull, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var report trivyReportFull
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, err
	}
	return &report, nil
}

// buildSiteImage creates a SiteImage from a Trivy report.
func buildSiteImage(id, originalRef, patchedRef, valuesPath string, report *trivyReportFull) SiteImage {
	osInfo := ""
	if report.Metadata.OS.Family != "" {
		osInfo = report.Metadata.OS.Family
		if report.Metadata.OS.Name != "" {
			osInfo += " " + report.Metadata.OS.Name
		}
	}

	vulns := make([]SiteVuln, 0)
	severityCounts := make(map[string]int)
	fixable := 0

	for _, result := range report.Results {
		for _, v := range result.Vulnerabilities {
			vuln := SiteVuln{
				ID:               v.VulnerabilityID,
				PkgName:          v.PkgName,
				InstalledVersion: v.InstalledVersion,
				FixedVersion:     v.FixedVersion,
				Severity:         v.Severity,
				Title:            v.Title,
			}
			vulns = append(vulns, vuln)
			sev := v.Severity
			if sev == "" {
				sev = "UNKNOWN"
			}
			severityCounts[sev]++
			if v.FixedVersion != "" {
				fixable++
			}
		}
	}

	return SiteImage{
		ID:          id,
		OriginalRef: originalRef,
		PatchedRef:  patchedRef,
		ValuesPath:  valuesPath,
		OS:          osInfo,
		VulnSummary: VulnSummary{
			Total:          len(vulns),
			Fixable:        fixable,
			SeverityCounts: severityCounts,
		},
		Vulnerabilities: vulns,
	}
}

// computeSummary aggregates stats across all images.
func computeSummary(allImages []SiteImage) SiteSummary {
	summary := SiteSummary{
		TotalImages: len(allImages),
	}
	for i := range allImages { // Use index to avoid copying 144 bytes
		img := &allImages[i]
		summary.TotalVulns += img.VulnSummary.Total
		summary.FixableVulns += img.VulnSummary.Fixable
	}

	return summary
}
