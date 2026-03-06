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
	GeneratedAt   string         `json:"generatedAt"`
	Registry      string         `json:"registry"`
	Summary       SiteSummary    `json:"summary"`
	Images        []SiteImage    `json:"images"`
	IntegerImages []IntegerImage `json:"integerImages"` // Wolfi-based rebuilds from integer pipeline
}

// IntegerImage describes a single image from github.com/verity-org/integer.
type IntegerImage struct {
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Versions    []IntegerVersion `json:"versions"`
}

// IntegerVersion is one version stream of an integer image (e.g. "1.26" for Go).
type IntegerVersion struct {
	Version  string           `json:"version"`
	Latest   bool             `json:"latest,omitempty"`
	EOL      string           `json:"eol,omitempty"`
	Variants []IntegerVariant `json:"variants"`
}

// IntegerVariant is one built type (default, dev, fips) within a version.
type IntegerVariant struct {
	Type    string   `json:"type"`
	Tags    []string `json:"tags"`
	Ref     string   `json:"ref"`
	Digest  string   `json:"digest"`
	BuiltAt string   `json:"builtAt"`
	Status  string   `json:"status"` // "success" | "failure" | "unknown"
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

// GenerateSiteData reads images.json and Trivy reports, returning a populated
// SiteData. Call MergeIntegerCatalog and WriteSiteData to finish the pipeline.
func GenerateSiteData(imagesJSON, reportsDir, postReportsDir, registry string) (*SiteData, error) {
	data, err := os.ReadFile(imagesJSON)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", imagesJSON, err)
	}

	var entries []ImageEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", imagesJSON, err)
	}

	siteData := &SiteData{
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
		if postReportsDir != "" && patchedRef != "" {
			postReportName := sanitize(patchedRef) + ".json"
			applyPostPatchReport(&si, filepath.Join(postReportsDir, postReportName))
		}

		allImages = append(allImages, si)
	}

	siteData.Images = allImages
	siteData.Summary = computeSummary(allImages)

	return siteData, nil
}

// WriteSiteData marshals siteData to JSON and writes it to outputPath.
func WriteSiteData(siteData *SiteData, outputPath string) error {
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}
	out, err := json.MarshalIndent(siteData, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling site data: %w", err)
	}
	return os.WriteFile(outputPath, out, 0o644)
}

// integerCatalog mirrors the schema produced by `integer catalog`.
type integerCatalog struct {
	Images []IntegerImage `json:"images"`
}

// MergeIntegerCatalog reads the catalog.json produced by the integer repo and
// populates siteData.IntegerImages. A missing or empty file is a no-op.
func MergeIntegerCatalog(siteData *SiteData, path string) error {
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("reading integer catalog %s: %w", path, err)
	}
	var cat integerCatalog
	if err := json.Unmarshal(data, &cat); err != nil {
		return fmt.Errorf("parsing integer catalog %s: %w", path, err)
	}
	siteData.IntegerImages = cat.Images
	return nil
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
