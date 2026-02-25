package internal

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateSiteDataFromJSON_WithReportsDir(t *testing.T) {
	// Create temp directory structure
	tmpDir := t.TempDir()
	reportsDir := filepath.Join(tmpDir, "reports")
	if err := os.MkdirAll(reportsDir, 0o755); err != nil {
		t.Fatalf("failed to create reports dir: %v", err)
	}

	// Create a sample Trivy report with one vulnerability
	trivyReport := map[string]any{
		"Metadata": map[string]any{
			"OS": map[string]any{
				"Family": "alpine",
				"Name":   "3.18",
			},
		},
		"Results": []map[string]any{
			{
				"Vulnerabilities": []map[string]any{
					{
						"VulnerabilityID":  "CVE-2024-TEST",
						"PkgName":          "test-pkg",
						"InstalledVersion": "1.0.0",
						"FixedVersion":     "1.0.1",
						"Severity":         "HIGH",
						"Title":            "Test vulnerability",
					},
				},
			},
		},
	}

	reportPath := filepath.Join(reportsDir, "docker.io_library_nginx_1.27.3.json")
	reportData, err := json.Marshal(trivyReport)
	if err != nil {
		t.Fatalf("failed to marshal report: %v", err)
	}
	if err := os.WriteFile(reportPath, reportData, 0o644); err != nil {
		t.Fatalf("failed to write report: %v", err)
	}

	// Create images.json with report filename (without reports/ prefix)
	imagesJSON := filepath.Join(tmpDir, "images.json")
	images := []ImageEntry{
		{
			Original: "docker.io/library/nginx:1.27.3",
			Patched:  "ghcr.io/verity-org/nginx:1.27.3-patched",
			Report:   "docker.io_library_nginx_1.27.3.json",
		},
	}
	imagesData, err := json.Marshal(images)
	if err != nil {
		t.Fatalf("failed to marshal images: %v", err)
	}
	if err := os.WriteFile(imagesJSON, imagesData, 0o644); err != nil {
		t.Fatalf("failed to write images.json: %v", err)
	}

	// Generate catalog
	outputPath := filepath.Join(tmpDir, "catalog.json")
	err = GenerateSiteDataFromJSON(imagesJSON, reportsDir, "ghcr.io/verity-org", outputPath)
	if err != nil {
		t.Fatalf("GenerateSiteDataFromJSON failed: %v", err)
	}

	// Read and verify catalog
	catalogData, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read catalog: %v", err)
	}

	var catalog SiteData
	if err := json.Unmarshal(catalogData, &catalog); err != nil {
		t.Fatalf("failed to unmarshal catalog: %v", err)
	}

	// Verify the report was read and vulnerability data is present
	if len(catalog.Images) != 1 {
		t.Fatalf("expected 1 image in catalog, got %d", len(catalog.Images))
	}

	img := catalog.Images[0]
	if img.VulnSummary.Total == 0 {
		t.Error("expected non-zero vulnerabilities (report should have been loaded)")
	}
	if img.VulnSummary.Total != 1 {
		t.Errorf("expected 1 vulnerability, got %d", img.VulnSummary.Total)
	}
	if len(img.Vulnerabilities) != 1 {
		t.Errorf("expected 1 vulnerability entry, got %d", len(img.Vulnerabilities))
	}
	if img.OS != "alpine 3.18" {
		t.Errorf("expected OS 'alpine 3.18', got '%s'", img.OS)
	}
}

func TestGenerateSiteDataFromJSON_FallbackWithoutReportsDir(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()

	// Create images.json with no report
	imagesJSON := filepath.Join(tmpDir, "images.json")
	images := []ImageEntry{
		{
			Original: "docker.io/library/nginx:1.27.3",
			Patched:  "ghcr.io/verity-org/nginx:1.27.3-patched",
			Report:   "",
		},
	}
	imagesData, err := json.Marshal(images)
	if err != nil {
		t.Fatalf("failed to marshal images: %v", err)
	}
	if err := os.WriteFile(imagesJSON, imagesData, 0o644); err != nil {
		t.Fatalf("failed to write images.json: %v", err)
	}

	// Generate catalog without reportsDir
	outputPath := filepath.Join(tmpDir, "catalog.json")
	err = GenerateSiteDataFromJSON(imagesJSON, "", "ghcr.io/verity-org", outputPath)
	if err != nil {
		t.Fatalf("GenerateSiteDataFromJSON failed: %v", err)
	}

	// Read and verify catalog
	catalogData, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read catalog: %v", err)
	}

	var catalog SiteData
	if err := json.Unmarshal(catalogData, &catalog); err != nil {
		t.Fatalf("failed to unmarshal catalog: %v", err)
	}

	// Verify fallback to zero vulnerabilities
	if len(catalog.Images) != 1 {
		t.Fatalf("expected 1 image in catalog, got %d", len(catalog.Images))
	}

	img := catalog.Images[0]
	if img.VulnSummary.Total != 0 {
		t.Errorf("expected zero vulnerabilities (no report), got %d", img.VulnSummary.Total)
	}
	if img.OriginalRef != "docker.io/library/nginx:1.27.3" {
		t.Errorf("wrong original ref: %s", img.OriginalRef)
	}
	if img.PatchedRef != "ghcr.io/verity-org/nginx:1.27.3-patched" {
		t.Errorf("wrong patched ref: %s", img.PatchedRef)
	}
}
