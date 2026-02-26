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
	Images      []SiteImage `json:"images"`
}

// SiteSummary aggregates stats across all images.
type SiteSummary struct {
	TotalImages      int `json:"totalImages"`
	TotalVulnsBefore int `json:"totalVulnsBefore"`
	TotalVulnsAfter  int `json:"totalVulnsAfter"`
	FixedVulns       int `json:"fixedVulns"`
}

// SiteImage describes a single container image with its vulnerability data.
type SiteImage struct {
	ID          string      `json:"id"`
	OriginalRef string      `json:"originalRef"`
	PatchedRef  string      `json:"patchedRef"`
	OS          string      `json:"os"`
	BeforeVulns VulnSummary `json:"beforeVulns"`
	AfterVulns  VulnSummary `json:"afterVulns"`
	// Remaining vulnerabilities after patching (from post-patch scan)
	Vulnerabilities []SiteVuln `json:"vulnerabilities"`
}

// VulnSummary counts vulnerabilities by severity.
type VulnSummary struct {
	Total          int            `json:"total"`
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

// ImageEntry represents a single image entry from the sign-and-attest.sh output.
type ImageEntry struct {
	Original string `json:"original"`
	Patched  string `json:"patched"`
	Report   string `json:"report"`
}

// GenerateSiteDataFromJSON reads images.json (from sign-and-attest.sh) to produce a catalog.json.
// reportsDir contains pre-patch Trivy reports; postReportsDir contains post-patch Trivy reports.
// If postReportsDir is empty, AfterVulns falls back to empty (showing only before data).
func GenerateSiteDataFromJSON(imagesJSON, reportsDir, postReportsDir, registry, outputPath string) error {
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
	}

	var allImages []SiteImage

	for _, entry := range entries {
		originalRef := entry.Original
		patchedRef := entry.Patched
		sanitizedRef := sanitize(originalRef)

		si := SiteImage{
			ID:              sanitizedRef,
			OriginalRef:     originalRef,
			PatchedRef:      patchedRef,
			BeforeVulns:     VulnSummary{SeverityCounts: make(map[string]int)},
			AfterVulns:      VulnSummary{SeverityCounts: make(map[string]int)},
			Vulnerabilities: []SiteVuln{},
		}

		// Pre-patch report — sets OS, BeforeVulns
		reportPath := entry.Report
		if reportsDir != "" && reportPath != "" {
			reportPath = filepath.Join(reportsDir, reportPath)
		}
		if reportPath != "" && fileExists(reportPath) {
			if report, err := parseTrivyReportFull(reportPath); err == nil {
				si.OS = osInfo(report)
				si.BeforeVulns = vulnSummary(report)
			} else {
				fmt.Fprintf(os.Stderr, "Warning: failed to parse pre-patch report %s: %v\n", reportPath, err)
			}
		}

		// Post-patch report — sets AfterVulns + remaining Vulnerabilities.
		// The post-scan job scans the patched image ref, so the report filename
		// is derived from patchedRef, not the source report name.
		if postReportsDir != "" && patchedRef != "" {
			postReportName := sanitize(patchedRef) + ".json"
			applyPostPatchReport(&si, filepath.Join(postReportsDir, postReportName))
		}

		allImages = append(allImages, si)
	}

	siteData.Images = allImages
	siteData.Summary = computeSummary(allImages)

	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	out, err := json.MarshalIndent(siteData, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling site data: %w", err)
	}
	return os.WriteFile(outputPath, out, 0o644)
}

// applyPostPatchReport populates AfterVulns and remaining Vulnerabilities on si
// from the post-patch Trivy report at postReportPath. No-ops if the file does
// not exist or cannot be parsed.
func applyPostPatchReport(si *SiteImage, postReportPath string) {
	if !fileExists(postReportPath) {
		return
	}
	report, err := parseTrivyReportFull(postReportPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to parse post-patch report %s: %v\n", postReportPath, err)
		return
	}
	si.AfterVulns = vulnSummary(report)
	si.Vulnerabilities = extractVulns(report)
}

// fileExists checks if a file exists at the given path.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

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

func osInfo(report *trivyReportFull) string {
	if report.Metadata.OS.Family == "" {
		return ""
	}
	if report.Metadata.OS.Name != "" {
		return report.Metadata.OS.Family + " " + report.Metadata.OS.Name
	}
	return report.Metadata.OS.Family
}

func vulnSummary(report *trivyReportFull) VulnSummary {
	counts := make(map[string]int)
	total := 0
	for _, result := range report.Results {
		for _, v := range result.Vulnerabilities {
			sev := v.Severity
			if sev == "" {
				sev = "UNKNOWN"
			}
			counts[sev]++
			total++
		}
	}
	return VulnSummary{Total: total, SeverityCounts: counts}
}

func extractVulns(report *trivyReportFull) []SiteVuln {
	var vulns []SiteVuln
	for _, result := range report.Results {
		for _, v := range result.Vulnerabilities {
			vulns = append(vulns, SiteVuln{
				ID:               v.VulnerabilityID,
				PkgName:          v.PkgName,
				InstalledVersion: v.InstalledVersion,
				FixedVersion:     v.FixedVersion,
				Severity:         v.Severity,
				Title:            v.Title,
			})
		}
	}
	if vulns == nil {
		return []SiteVuln{}
	}
	return vulns
}

func computeSummary(allImages []SiteImage) SiteSummary {
	summary := SiteSummary{TotalImages: len(allImages)}
	for i := range allImages {
		img := &allImages[i]
		summary.TotalVulnsBefore += img.BeforeVulns.Total
		summary.TotalVulnsAfter += img.AfterVulns.Total
	}
	summary.FixedVulns = summary.TotalVulnsBefore - summary.TotalVulnsAfter
	return summary
}
