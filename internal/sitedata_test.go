package internal

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateSiteDataFromJSON_WithReportsDir(t *testing.T) {
	tmpDir := t.TempDir()
	reportsDir := filepath.Join(tmpDir, "reports")
	if err := os.MkdirAll(reportsDir, 0o755); err != nil {
		t.Fatalf("failed to create reports dir: %v", err)
	}

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

	reportName := "docker.io_library_nginx_1.27.3.json"
	reportPath := filepath.Join(reportsDir, reportName)
	reportData, err := json.Marshal(trivyReport)
	if err != nil {
		t.Fatalf("failed to marshal report: %v", err)
	}
	if err := os.WriteFile(reportPath, reportData, 0o644); err != nil {
		t.Fatalf("failed to write report: %v", err)
	}

	imagesJSON := filepath.Join(tmpDir, "images.json")
	images := []ImageEntry{
		{
			Original: "docker.io/library/nginx:1.27.3",
			Patched:  "ghcr.io/verity-org/nginx:1.27.3-patched",
			Report:   reportName,
		},
	}
	imagesData, err := json.Marshal(images)
	if err != nil {
		t.Fatalf("failed to marshal images: %v", err)
	}
	if err := os.WriteFile(imagesJSON, imagesData, 0o644); err != nil {
		t.Fatalf("failed to write images.json: %v", err)
	}

	outputPath := filepath.Join(tmpDir, "catalog.json")
	err = GenerateSiteDataFromJSON(imagesJSON, reportsDir, "", "ghcr.io/verity-org", outputPath)
	if err != nil {
		t.Fatalf("GenerateSiteDataFromJSON failed: %v", err)
	}

	catalogData, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read catalog: %v", err)
	}

	var catalog SiteData
	if err := json.Unmarshal(catalogData, &catalog); err != nil {
		t.Fatalf("failed to unmarshal catalog: %v", err)
	}

	if len(catalog.Images) != 1 {
		t.Fatalf("expected 1 image in catalog, got %d", len(catalog.Images))
	}

	img := catalog.Images[0]
	if img.BeforeVulns.Total != 1 {
		t.Errorf("expected 1 before vulnerability, got %d", img.BeforeVulns.Total)
	}
	if img.AfterVulns.Total != 0 {
		t.Errorf("expected 0 after vulnerabilities (no post report), got %d", img.AfterVulns.Total)
	}
	if img.OS != "alpine 3.18" {
		t.Errorf("expected OS 'alpine 3.18', got '%s'", img.OS)
	}
	if catalog.Summary.TotalVulnsBefore != 1 {
		t.Errorf("expected summary.totalVulnsBefore = 1, got %d", catalog.Summary.TotalVulnsBefore)
	}
	if catalog.Summary.FixedVulns != 1 {
		t.Errorf("expected summary.fixedVulns = 1, got %d", catalog.Summary.FixedVulns)
	}
}

func TestGenerateSiteDataFromJSON_BeforeAfter(t *testing.T) {
	tmpDir := t.TempDir()
	preDir := filepath.Join(tmpDir, "pre")
	postDir := filepath.Join(tmpDir, "post")
	for _, d := range []string{preDir, postDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("failed to create dir: %v", err)
		}
	}

	reportName := "docker.io_library_nginx_1.27.3.json"

	// Pre-patch: 3 vulns
	preReport := map[string]any{
		"Metadata": map[string]any{"OS": map[string]any{"Family": "debian", "Name": "12"}},
		"Results": []map[string]any{
			{"Vulnerabilities": []map[string]any{
				{"VulnerabilityID": "CVE-A", "PkgName": "pkgA", "Severity": "CRITICAL"},
				{"VulnerabilityID": "CVE-B", "PkgName": "pkgB", "Severity": "HIGH"},
				{"VulnerabilityID": "CVE-C", "PkgName": "pkgC", "Severity": "LOW"},
			}},
		},
	}
	// Post-patch: 1 vuln remaining
	postReport := map[string]any{
		"Metadata": map[string]any{"OS": map[string]any{"Family": "debian", "Name": "12"}},
		"Results": []map[string]any{
			{"Vulnerabilities": []map[string]any{
				{"VulnerabilityID": "CVE-C", "PkgName": "pkgC", "Severity": "LOW"},
			}},
		},
	}

	for _, tc := range []struct {
		dir  string
		data any
	}{
		{preDir, preReport},
		{postDir, postReport},
	} {
		d, err := json.Marshal(tc.data)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(tc.dir, reportName), d, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	imagesJSON := filepath.Join(tmpDir, "images.json")
	images := []ImageEntry{{
		Original: "docker.io/library/nginx:1.27.3",
		Patched:  "ghcr.io/verity-org/nginx:1.27.3-patched",
		Report:   reportName,
	}}
	d, err := json.Marshal(images)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(imagesJSON, d, 0o644); err != nil {
		t.Fatal(err)
	}

	outputPath := filepath.Join(tmpDir, "catalog.json")
	if err := GenerateSiteDataFromJSON(imagesJSON, preDir, postDir, "ghcr.io/verity-org", outputPath); err != nil {
		t.Fatalf("GenerateSiteDataFromJSON failed: %v", err)
	}

	var catalog SiteData
	catalogData, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read catalog: %v", err)
	}
	if err := json.Unmarshal(catalogData, &catalog); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	img := catalog.Images[0]
	if img.BeforeVulns.Total != 3 {
		t.Errorf("BeforeVulns.Total = %d, want 3", img.BeforeVulns.Total)
	}
	if img.AfterVulns.Total != 1 {
		t.Errorf("AfterVulns.Total = %d, want 1", img.AfterVulns.Total)
	}
	if len(img.Vulnerabilities) != 1 {
		t.Errorf("Vulnerabilities len = %d, want 1", len(img.Vulnerabilities))
	}
	if img.Vulnerabilities[0].ID != "CVE-C" {
		t.Errorf("remaining vuln ID = %s, want CVE-C", img.Vulnerabilities[0].ID)
	}
	if catalog.Summary.FixedVulns != 2 {
		t.Errorf("summary.fixedVulns = %d, want 2", catalog.Summary.FixedVulns)
	}
}

func TestGenerateSiteDataFromJSON_FallbackWithoutReportsDir(t *testing.T) {
	tmpDir := t.TempDir()

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

	outputPath := filepath.Join(tmpDir, "catalog.json")
	err = GenerateSiteDataFromJSON(imagesJSON, "", "", "ghcr.io/verity-org", outputPath)
	if err != nil {
		t.Fatalf("GenerateSiteDataFromJSON failed: %v", err)
	}

	catalogData, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read catalog: %v", err)
	}

	var catalog SiteData
	if err := json.Unmarshal(catalogData, &catalog); err != nil {
		t.Fatalf("failed to unmarshal catalog: %v", err)
	}

	if len(catalog.Images) != 1 {
		t.Fatalf("expected 1 image in catalog, got %d", len(catalog.Images))
	}

	img := catalog.Images[0]
	if img.BeforeVulns.Total != 0 {
		t.Errorf("expected zero before vulnerabilities (no report), got %d", img.BeforeVulns.Total)
	}
	if img.OriginalRef != "docker.io/library/nginx:1.27.3" {
		t.Errorf("wrong original ref: %s", img.OriginalRef)
	}
	if img.PatchedRef != "ghcr.io/verity-org/nginx:1.27.3-patched" {
		t.Errorf("wrong patched ref: %s", img.PatchedRef)
	}
}
