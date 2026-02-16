package internal

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateSiteData(t *testing.T) {
	tmpDir := t.TempDir()

	// Write an images file
	imagesFile := filepath.Join(tmpDir, "images.yaml")
	imagesYaml := `redis:
  image:
    registry: docker.io
    repository: library/redis
    tag: "7.0.0"
`
	if err := os.WriteFile(imagesFile, []byte(imagesYaml), 0o644); err != nil {
		t.Fatal(err)
	}

	// Run GenerateSiteData (no registry)
	outputPath := filepath.Join(tmpDir, "output", "catalog.json")
	err := GenerateSiteData(imagesFile, "", "", outputPath)
	if err != nil {
		t.Fatalf("GenerateSiteData failed: %v", err)
	}

	// Read and parse output
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read output: %v", err)
	}

	var siteData SiteData
	if err := json.Unmarshal(data, &siteData); err != nil {
		t.Fatalf("Failed to parse output JSON: %v", err)
	}

	// Verify structure
	if siteData.Registry != "" {
		t.Errorf("expected empty registry, got %s", siteData.Registry)
	}
	if siteData.GeneratedAt == "" {
		t.Error("expected generatedAt to be set")
	}

	// Verify images list
	if len(siteData.Images) != 1 {
		t.Fatalf("expected 1 image, got %d", len(siteData.Images))
	}
	img := siteData.Images[0]
	if img.OriginalRef != "docker.io/library/redis:7.0.0" {
		t.Errorf("expected redis image, got %s", img.OriginalRef)
	}

	// Verify summary
	if siteData.Summary.TotalImages != 1 {
		t.Errorf("expected 1 total image, got %d", siteData.Summary.TotalImages)
	}
}

func TestParseTrivyReportFull(t *testing.T) {
	tmpDir := t.TempDir()
	reportJSON := `{
		"Metadata": {"OS": {"Family": "debian", "Name": "11.5"}},
		"Results": [
			{
				"Vulnerabilities": [
					{"VulnerabilityID":"CVE-2024-0001","PkgName":"openssl","InstalledVersion":"1.1.1","FixedVersion":"1.1.2","Severity":"HIGH","Title":"test vuln"},
					{"VulnerabilityID":"CVE-2024-0002","PkgName":"zlib","InstalledVersion":"1.2.11","FixedVersion":"","Severity":"","Title":"unknown sev"}
				]
			}
		]
	}`
	reportPath := filepath.Join(tmpDir, "report.json")
	if err := os.WriteFile(reportPath, []byte(reportJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	report, err := parseTrivyReportFull(reportPath)
	if err != nil {
		t.Fatalf("parseTrivyReportFull failed: %v", err)
	}

	if report.Metadata.OS.Family != "debian" {
		t.Errorf("expected OS family debian, got %s", report.Metadata.OS.Family)
	}
	if report.Metadata.OS.Name != "11.5" {
		t.Errorf("expected OS name 11.5, got %s", report.Metadata.OS.Name)
	}
	if len(report.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(report.Results))
	}
	if len(report.Results[0].Vulnerabilities) != 2 {
		t.Fatalf("expected 2 vulns, got %d", len(report.Results[0].Vulnerabilities))
	}
	if report.Results[0].Vulnerabilities[0].Severity != "HIGH" {
		t.Errorf("expected HIGH, got %s", report.Results[0].Vulnerabilities[0].Severity)
	}
}

func TestGenerateSiteData_ChartsFieldPresent(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a minimal values.yaml (no chart data, just standalone images)
	imagesFile := filepath.Join(tmpDir, "values.yaml")
	if err := os.WriteFile(imagesFile, []byte(`nginx:
  image:
    registry: docker.io
    repository: library/nginx
    tag: "1.25"
`), 0o644); err != nil {
		t.Fatal(err)
	}

	outputPath := filepath.Join(tmpDir, "catalog.json")
	if err := GenerateSiteData(imagesFile, "", "ghcr.io/test", outputPath); err != nil {
		t.Fatalf("GenerateSiteData failed: %v", err)
	}

	// Read raw JSON and verify "charts" key exists and is an array (not null/missing)
	raw, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}

	var parsed map[string]json.RawMessage
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	chartsRaw, ok := parsed["charts"]
	if !ok {
		t.Fatal("catalog JSON missing 'charts' key — Astro site will crash with 'Cannot read properties of undefined'")
	}

	// Ensure it's an array, not null
	if string(chartsRaw) == "null" {
		t.Fatal("catalog JSON has 'charts': null — Astro site will crash calling .map() on null")
	}

	var charts []json.RawMessage
	if err := json.Unmarshal(chartsRaw, &charts); err != nil {
		t.Fatalf("'charts' is not a JSON array: %v", err)
	}
}

func TestComputeSummary(t *testing.T) {
	allImages := []SiteImage{
		{VulnSummary: VulnSummary{Total: 5, Fixable: 3, SeverityCounts: map[string]int{"HIGH": 3, "LOW": 2}}},
		{VulnSummary: VulnSummary{Total: 2, Fixable: 1, SeverityCounts: map[string]int{"MEDIUM": 2}}},
		{VulnSummary: VulnSummary{Total: 1, Fixable: 0, SeverityCounts: map[string]int{"LOW": 1}}},
	}

	summary := computeSummary(allImages)

	if summary.TotalImages != 3 {
		t.Errorf("expected 3 images, got %d", summary.TotalImages)
	}
	if summary.TotalVulns != 8 {
		t.Errorf("expected 8 vulns, got %d", summary.TotalVulns)
	}
	if summary.FixableVulns != 4 {
		t.Errorf("expected 4 fixable, got %d", summary.FixableVulns)
	}
}
