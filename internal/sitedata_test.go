package internal

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestGenerateSiteData(t *testing.T) {
	// Create a mock chart structure in a temp dir
	tmpDir := t.TempDir()
	chartsDir := filepath.Join(tmpDir, "charts")
	chartDir := filepath.Join(chartsDir, "myapp")
	reportsDir := filepath.Join(chartDir, "reports")

	if err := os.MkdirAll(reportsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Write Chart.yaml
	chartYaml := `apiVersion: v2
name: myapp
description: myapp with Copa-patched container images
type: application
version: 1.0.0-0
dependencies:
    - name: myapp
      version: "1.0.0"
      repository: oci://ghcr.io/example/charts
`
	if err := os.WriteFile(filepath.Join(chartDir, "Chart.yaml"), []byte(chartYaml), 0o644); err != nil {
		t.Fatal(err)
	}

	// Write values.yaml
	valuesYaml := `myapp:
    image:
        registry: ghcr.io/testorg
        repository: myorg/myapp
        tag: v1.0.0-patched
`
	if err := os.WriteFile(filepath.Join(chartDir, "values.yaml"), []byte(valuesYaml), 0o644); err != nil {
		t.Fatal(err)
	}

	// Write a mock Trivy report
	report := map[string]interface{}{
		"Metadata": map[string]interface{}{
			"OS": map[string]interface{}{
				"Family": "debian",
				"Name":   "12.0",
			},
		},
		"Results": []map[string]interface{}{
			{
				"Vulnerabilities": []map[string]interface{}{
					{
						"VulnerabilityID":  "CVE-2024-0001",
						"PkgName":          "openssl",
						"InstalledVersion": "1.1.1",
						"FixedVersion":     "1.1.2",
						"Severity":         "HIGH",
						"Title":            "Buffer overflow in openssl",
					},
					{
						"VulnerabilityID":  "CVE-2024-0002",
						"PkgName":          "zlib",
						"InstalledVersion": "1.2.11",
						"FixedVersion":     "1.2.12",
						"Severity":         "MEDIUM",
						"Title":            "Integer overflow in zlib",
					},
				},
			},
		},
	}
	reportJSON, _ := json.Marshal(report)
	reportPath := filepath.Join(reportsDir, "docker.io_myorg_myapp_v1.0.0.json")
	if err := os.WriteFile(reportPath, reportJSON, 0o644); err != nil {
		t.Fatal(err)
	}

	// Write a standalone images file
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

	// Run GenerateSiteData
	outputPath := filepath.Join(tmpDir, "output", "catalog.json")
	err := GenerateSiteData(chartsDir, imagesFile, filepath.Join(tmpDir, "reports"), "ghcr.io/testorg", outputPath)
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
	if siteData.Registry != "ghcr.io/testorg" {
		t.Errorf("expected registry ghcr.io/testorg, got %s", siteData.Registry)
	}
	if siteData.GeneratedAt == "" {
		t.Error("expected generatedAt to be set")
	}

	// Verify charts
	if len(siteData.Charts) != 1 {
		t.Fatalf("expected 1 chart, got %d", len(siteData.Charts))
	}
	chart := siteData.Charts[0]
	if chart.Name != "myapp" {
		t.Errorf("expected chart name myapp, got %s", chart.Name)
	}
	if chart.Version != "1.0.0-0" {
		t.Errorf("expected version 1.0.0-0, got %s", chart.Version)
	}
	if chart.UpstreamVersion != "1.0.0" {
		t.Errorf("expected upstream version 1.0.0, got %s", chart.UpstreamVersion)
	}

	// Verify chart images
	if len(chart.Images) != 1 {
		t.Fatalf("expected 1 image, got %d", len(chart.Images))
	}
	img := chart.Images[0]
	if img.OriginalRef != "docker.io/myorg/myapp:v1.0.0" {
		t.Errorf("expected original ref docker.io/myorg/myapp:v1.0.0, got %s", img.OriginalRef)
	}
	if img.OS != "debian 12.0" {
		t.Errorf("expected OS debian 12.0, got %s", img.OS)
	}
	if img.VulnSummary.Total != 2 {
		t.Errorf("expected 2 total vulns, got %d", img.VulnSummary.Total)
	}
	if img.VulnSummary.Fixable != 2 {
		t.Errorf("expected 2 fixable vulns, got %d", img.VulnSummary.Fixable)
	}
	if img.VulnSummary.SeverityCounts["HIGH"] != 1 {
		t.Errorf("expected 1 HIGH vuln, got %d", img.VulnSummary.SeverityCounts["HIGH"])
	}
	if img.VulnSummary.SeverityCounts["MEDIUM"] != 1 {
		t.Errorf("expected 1 MEDIUM vuln, got %d", img.VulnSummary.SeverityCounts["MEDIUM"])
	}

	// Verify standalone images
	if len(siteData.StandaloneImages) != 1 {
		t.Fatalf("expected 1 standalone image, got %d", len(siteData.StandaloneImages))
	}
	si := siteData.StandaloneImages[0]
	if si.OriginalRef != "docker.io/library/redis:7.0.0" {
		t.Errorf("expected original ref docker.io/library/redis:7.0.0, got %s", si.OriginalRef)
	}

	// Verify summary
	if siteData.Summary.TotalCharts != 1 {
		t.Errorf("expected 1 total chart, got %d", siteData.Summary.TotalCharts)
	}
	if siteData.Summary.TotalImages != 2 {
		t.Errorf("expected 2 total images, got %d", siteData.Summary.TotalImages)
	}
}

func TestParseTrivyReportFull(t *testing.T) {
	reportPath := filepath.Join("..", "charts", "prometheus", "reports", "quay.io_brancz_kube-rbac-proxy_v0.14.0.json")

	// Skip if real report doesn't exist (CI without charts dir)
	if _, err := os.Stat(reportPath); os.IsNotExist(err) {
		t.Skip("skipping: real trivy report not found")
	}

	report, err := parseTrivyReportFull(reportPath)
	if err != nil {
		t.Fatalf("parseTrivyReportFull failed: %v", err)
	}

	// Verify OS metadata
	if report.Metadata.OS.Family != "debian" {
		t.Errorf("expected OS family debian, got %s", report.Metadata.OS.Family)
	}
	if report.Metadata.OS.Name != "11.5" {
		t.Errorf("expected OS name 11.5, got %s", report.Metadata.OS.Name)
	}

	// Verify vulnerabilities exist
	if len(report.Results) == 0 {
		t.Fatal("expected at least one result")
	}

	totalVulns := 0
	severityCounts := make(map[string]int)
	for _, r := range report.Results {
		for _, v := range r.Vulnerabilities {
			totalVulns++
			sev := v.Severity
			if sev == "" {
				sev = "UNKNOWN"
			}
			severityCounts[sev]++
		}
	}

	if totalVulns == 0 {
		t.Error("expected at least one vulnerability")
	}

	// The kube-rbac-proxy v0.14.0 report should have UNKNOWN severity vulns
	if severityCounts["UNKNOWN"] == 0 {
		t.Error("expected UNKNOWN severity vulnerabilities")
	}

	t.Logf("Report: %d total vulns, severity counts: %v", totalVulns, severityCounts)
}

func TestUnsanitize(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "quay.io_brancz_kube-rbac-proxy_v0.14.0",
			expected: "quay.io/brancz/kube-rbac-proxy:v0.14.0",
		},
		{
			input:    "quay.io_prometheus_prometheus_v2.48.0",
			expected: "quay.io/prometheus/prometheus:v2.48.0",
		},
		{
			input:    "registry.k8s.io_kube-state-metrics_kube-state-metrics_v2.10.1",
			expected: "registry.k8s.io/kube-state-metrics/kube-state-metrics:v2.10.1",
		},
		{
			input:    "docker.io_grafana_grafana_12.3.1",
			expected: "docker.io/grafana/grafana:12.3.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := unsanitize(tt.input)
			if result != tt.expected {
				t.Errorf("unsanitize(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSanitizeRoundTrip(t *testing.T) {
	refs := []string{
		"quay.io/brancz/kube-rbac-proxy:v0.14.0",
		"quay.io/prometheus/prometheus:v2.48.0",
		"docker.io/grafana/grafana:12.3.1",
	}

	for _, ref := range refs {
		t.Run(ref, func(t *testing.T) {
			sanitized := sanitize(ref)
			restored := unsanitize(sanitized)
			if restored != ref {
				t.Errorf("round-trip failed: %q → %q → %q", ref, sanitized, restored)
			}
		})
	}
}

func TestSaveStandaloneReports(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a fake report file
	srcDir := filepath.Join(tmpDir, "src")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	reportContent := `{"Metadata":{"OS":{"Family":"debian","Name":"12"}},"Results":[{"Vulnerabilities":[{"VulnerabilityID":"CVE-2024-9999","PkgName":"curl","InstalledVersion":"7.88","FixedVersion":"7.89","Severity":"HIGH","Title":"test vuln"}]}]}`
	srcReport := filepath.Join(srcDir, "report.json")
	if err := os.WriteFile(srcReport, []byte(reportContent), 0o644); err != nil {
		t.Fatal(err)
	}

	results := []*PatchResult{
		{
			Original:   Image{Registry: "docker.io", Repository: "library/redis", Tag: "7.0.0"},
			ReportPath: srcReport,
		},
		{
			Original:   Image{Registry: "quay.io", Repository: "prometheus/prometheus", Tag: "v3.0.0"},
			ReportPath: "", // no report
		},
	}

	destDir := filepath.Join(tmpDir, "reports")
	if err := SaveStandaloneReports(results, destDir); err != nil {
		t.Fatalf("SaveStandaloneReports failed: %v", err)
	}

	// Check the report was saved with the correct filename
	expectedFile := filepath.Join(destDir, "docker.io_library_redis_7.0.0.json")
	if _, err := os.Stat(expectedFile); os.IsNotExist(err) {
		t.Errorf("expected report at %s, not found", expectedFile)
	}

	// Verify content is intact
	data, err := os.ReadFile(expectedFile)
	if err != nil {
		t.Fatal(err)
	}
	report, err := parseTrivyReportFull(expectedFile)
	if err != nil {
		t.Fatalf("failed to parse saved report: %v", err)
	}
	if report.Metadata.OS.Family != "debian" {
		t.Errorf("expected debian, got %s", report.Metadata.OS.Family)
	}
	_ = data

	// Image with no report should be skipped
	missingFile := filepath.Join(destDir, "quay.io_prometheus_prometheus_v3.0.0.json")
	if _, err := os.Stat(missingFile); !os.IsNotExist(err) {
		t.Errorf("expected no report for image without ReportPath, but file exists")
	}
}

func TestGenerateSiteDataWithStandaloneReports(t *testing.T) {
	tmpDir := t.TempDir()
	chartsDir := filepath.Join(tmpDir, "charts")
	reportsDir := filepath.Join(tmpDir, "reports")

	if err := os.MkdirAll(reportsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Write a standalone report
	report := `{"Metadata":{"OS":{"Family":"alpine","Name":"3.19"}},"Results":[{"Vulnerabilities":[{"VulnerabilityID":"CVE-2024-1111","PkgName":"busybox","InstalledVersion":"1.36.0","FixedVersion":"1.36.1","Severity":"MEDIUM","Title":"busybox overflow"}]}]}`
	if err := os.WriteFile(filepath.Join(reportsDir, "docker.io_library_redis_7.0.0.json"), []byte(report), 0o644); err != nil {
		t.Fatal(err)
	}

	// Write standalone images file
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

	outputPath := filepath.Join(tmpDir, "catalog.json")
	if err := GenerateSiteData(chartsDir, imagesFile, reportsDir, "ghcr.io/testorg", outputPath); err != nil {
		t.Fatalf("GenerateSiteData failed: %v", err)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	var siteData SiteData
	if err := json.Unmarshal(data, &siteData); err != nil {
		t.Fatal(err)
	}

	if len(siteData.StandaloneImages) != 1 {
		t.Fatalf("expected 1 standalone image, got %d", len(siteData.StandaloneImages))
	}

	si := siteData.StandaloneImages[0]
	if si.VulnSummary.Total != 1 {
		t.Errorf("expected 1 vuln, got %d", si.VulnSummary.Total)
	}
	if si.OS != "alpine 3.19" {
		t.Errorf("expected OS alpine 3.19, got %s", si.OS)
	}
	if si.VulnSummary.SeverityCounts["MEDIUM"] != 1 {
		t.Errorf("expected 1 MEDIUM, got %d", si.VulnSummary.SeverityCounts["MEDIUM"])
	}
}

func TestComputeSummary(t *testing.T) {
	charts := []SiteChart{
		{
			Name: "chart1",
			Images: []SiteImage{
				{VulnSummary: VulnSummary{Total: 5, Fixable: 3, SeverityCounts: map[string]int{"HIGH": 3, "LOW": 2}}},
				{VulnSummary: VulnSummary{Total: 2, Fixable: 1, SeverityCounts: map[string]int{"MEDIUM": 2}}},
			},
		},
	}
	standalone := []SiteImage{
		{VulnSummary: VulnSummary{Total: 1, Fixable: 0, SeverityCounts: map[string]int{"LOW": 1}}},
	}

	summary := computeSummary(charts, standalone)

	if summary.TotalCharts != 1 {
		t.Errorf("expected 1 chart, got %d", summary.TotalCharts)
	}
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

func TestComputeSummaryMultipleVersions(t *testing.T) {
	// Multiple chart entries with the same name (different versions)
	// should count as 1 unique chart.
	charts := []SiteChart{
		{
			Name:    "prometheus",
			Version: "28.9.1-5",
			Images: []SiteImage{
				{VulnSummary: VulnSummary{Total: 4, Fixable: 4, SeverityCounts: map[string]int{"UNKNOWN": 4}}},
			},
		},
		{
			Name:    "prometheus",
			Version: "28.9.1-4",
			Images: []SiteImage{
				{VulnSummary: VulnSummary{Total: 2, Fixable: 2, SeverityCounts: map[string]int{"HIGH": 2}}},
			},
		},
		{
			Name:    "victoria-logs-single",
			Version: "0.11.24-1",
			Images: []SiteImage{
				{VulnSummary: VulnSummary{Total: 1, Fixable: 0, SeverityCounts: map[string]int{"LOW": 1}}},
			},
		},
	}

	summary := computeSummary(charts, nil)

	if summary.TotalCharts != 2 {
		t.Errorf("expected 2 unique charts, got %d", summary.TotalCharts)
	}
	if summary.TotalImages != 3 {
		t.Errorf("expected 3 images, got %d", summary.TotalImages)
	}
	if summary.TotalVulns != 7 {
		t.Errorf("expected 7 vulns, got %d", summary.TotalVulns)
	}
	if summary.FixableVulns != 6 {
		t.Errorf("expected 6 fixable, got %d", summary.FixableVulns)
	}
}

// Integration tests against the public ghcr.io/descope registry.

func TestListGitHubPackageTags(t *testing.T) {
	if os.Getenv("GITHUB_TOKEN") == "" {
		t.Skip("GITHUB_TOKEN not set")
	}

	tags, err := listGitHubPackageTags("ghcr.io/descope", "prometheus")
	if err != nil {
		t.Fatalf("listGitHubPackageTags failed: %v", err)
	}

	if len(tags) == 0 {
		t.Fatal("expected at least one tag, got none")
	}

	// The registry should have multiple versions of prometheus.
	// Verify we see at least 2 tags and that known versions exist.
	t.Logf("found %d tags: %v", len(tags), tags)

	found := make(map[string]bool)
	for _, tag := range tags {
		found[tag] = true
	}

	// These versions are known to be published.
	for _, expected := range []string{"25.8.0-0", "28.9.1-5"} {
		if !found[expected] {
			t.Errorf("expected tag %q not found in %v", expected, tags)
		}
	}
}

func TestListChartTags(t *testing.T) {
	if os.Getenv("GITHUB_TOKEN") == "" {
		t.Skip("GITHUB_TOKEN not set")
	}

	tags, err := listChartTags("ghcr.io/descope", "victoria-logs-single")
	if err != nil {
		t.Fatalf("listChartTags failed: %v", err)
	}

	if len(tags) < 1 {
		t.Fatal("expected at least one tag")
	}

	found := false
	for _, tag := range tags {
		if tag == "0.11.24-1" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected tag 0.11.24-1 in %v", tags)
	}
}

func TestDiscoverRegistryVersions(t *testing.T) {
	if os.Getenv("GITHUB_TOKEN") == "" {
		t.Skip("GITHUB_TOKEN not set")
	}

	// Discover all versions except the "local" one (28.9.1-5).
	charts, err := discoverRegistryVersions(
		"prometheus",
		"28.9.1-5",
		"oci://ghcr.io/descope/charts",
		"ghcr.io/descope",
	)
	if err != nil {
		t.Fatalf("discoverRegistryVersions failed: %v", err)
	}

	// Should find historical versions (at least the 25.8.0-x series).
	if len(charts) == 0 {
		t.Fatal("expected at least one historical version, got none")
	}

	t.Logf("discovered %d historical versions:", len(charts))
	for _, c := range charts {
		t.Logf("  %s v%s (upstream %s) — %d images",
			c.Name, c.Version, c.UpstreamVersion, len(c.Images))
	}

	// Each chart entry should have a non-empty name, version, and images.
	for _, c := range charts {
		if c.Name != "prometheus" {
			t.Errorf("expected name 'prometheus', got %q", c.Name)
		}
		if c.Version == "" {
			t.Error("expected non-empty version")
		}
		if c.Version == "28.9.1-5" {
			t.Error("local version should be excluded")
		}
		if len(c.Images) == 0 {
			t.Errorf("version %s has no images", c.Version)
		}
	}

	// Verify that a 25.8.0-x version exists with different images
	// than the 28.9.1 series (confirming per-version filtering works).
	var found25 *SiteChart
	for i := range charts {
		if charts[i].UpstreamVersion == "25.8.0" {
			found25 = &charts[i]
			break
		}
	}
	if found25 == nil {
		t.Fatal("expected to find a 25.8.0-x version")
	}

	// The 25.8.0 series should have different image tags than 28.9.1.
	imageRefs := make([]string, 0, len(found25.Images))
	for _, img := range found25.Images {
		imageRefs = append(imageRefs, img.OriginalRef)
	}
	sort.Strings(imageRefs)
	t.Logf("25.8.0 images: %v", imageRefs)

	// Verify it does NOT contain 28.9.1-era images.
	for _, ref := range imageRefs {
		if ref == "quay.io/prometheus/prometheus:v3.9.1" {
			t.Error("25.8.0 version should not contain v3.9.1 images")
		}
	}
}

func TestDiscoverRegistryVersionsNonExistent(t *testing.T) {
	if os.Getenv("GITHUB_TOKEN") == "" {
		t.Skip("GITHUB_TOKEN not set")
	}

	// Non-existent chart should return empty, not error.
	charts, err := discoverRegistryVersions(
		"nonexistent-chart-xyz",
		"1.0.0",
		"oci://ghcr.io/descope/charts",
		"ghcr.io/descope",
	)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if len(charts) != 0 {
		t.Errorf("expected 0 charts, got %d", len(charts))
	}
}
