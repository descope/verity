package internal

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestGenerateMatrix(t *testing.T) {
	manifest := &DiscoveryManifest{
		Charts: []ChartDiscovery{
			{
				Name:    "prometheus",
				Version: "28.9.1",
				Images: []ImageDiscovery{
					{Registry: "quay.io", Repository: "prometheus/prometheus", Tag: "v3.9.1", Path: "server.image"},
					{Registry: "quay.io", Repository: "prometheus/alertmanager", Tag: "v0.27.0", Path: "alertmanager.image"},
				},
			},
		},
		Images: []ImageDiscovery{
			{Registry: "quay.io", Repository: "prometheus/prometheus", Tag: "v3.9.1", Path: "server.image"},
			{Registry: "quay.io", Repository: "prometheus/alertmanager", Tag: "v0.27.0", Path: "alertmanager.image"},
			{Registry: "docker.io", Repository: "grafana/grafana", Tag: "12.3.3", Path: "grafana.image"},
			// Duplicate of a chart image — should be deduplicated.
			{Registry: "quay.io", Repository: "prometheus/prometheus", Tag: "v3.9.1", Path: "prometheus.image"},
		},
	}

	matrix := GenerateMatrix(manifest)

	if len(matrix.Include) != 3 {
		t.Fatalf("expected 3 matrix entries (deduplicated), got %d", len(matrix.Include))
	}

	refs := map[string]bool{}
	for _, e := range matrix.Include {
		refs[e.ImageRef] = true
		if e.ImageName == "" {
			t.Errorf("ImageName should not be empty for %s", e.ImageRef)
		}
	}

	expected := []string{
		"quay.io/prometheus/prometheus:v3.9.1",
		"quay.io/prometheus/alertmanager:v0.27.0",
		"docker.io/grafana/grafana:12.3.3",
	}
	for _, e := range expected {
		if !refs[e] {
			t.Errorf("expected %q in matrix, got %v", e, refs)
		}
	}
}

func TestGenerateMatrixEmpty(t *testing.T) {
	manifest := &DiscoveryManifest{}
	matrix := GenerateMatrix(manifest)

	if len(matrix.Include) != 0 {
		t.Errorf("expected empty matrix, got %d entries", len(matrix.Include))
	}
}

func TestWriteDiscoveryOutput(t *testing.T) {
	dir := t.TempDir()

	manifest := &DiscoveryManifest{
		Charts: []ChartDiscovery{
			{
				Name:       "nginx",
				Version:    "1.0.0",
				Repository: "oci://ghcr.io/charts",
				Images: []ImageDiscovery{
					{Registry: "docker.io", Repository: "library/nginx", Tag: "1.25", Path: "image"},
				},
			},
		},
		Images: []ImageDiscovery{
			{Registry: "docker.io", Repository: "library/nginx", Tag: "1.25", Path: "image"},
		},
	}
	matrix := GenerateMatrix(manifest)

	if err := WriteDiscoveryOutput(manifest, matrix, dir); err != nil {
		t.Fatalf("WriteDiscoveryOutput() error: %v", err)
	}

	// Check manifest.json
	mData, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatalf("reading manifest.json: %v", err)
	}
	var gotManifest DiscoveryManifest
	if err := json.Unmarshal(mData, &gotManifest); err != nil {
		t.Fatalf("parsing manifest.json: %v", err)
	}
	if len(gotManifest.Charts) != 1 {
		t.Errorf("expected 1 chart in manifest, got %d", len(gotManifest.Charts))
	}
	if gotManifest.Charts[0].Name != "nginx" {
		t.Errorf("expected chart name 'nginx', got %q", gotManifest.Charts[0].Name)
	}

	// Check matrix.json is compact (single line)
	matData, err := os.ReadFile(filepath.Join(dir, "matrix.json"))
	if err != nil {
		t.Fatalf("reading matrix.json: %v", err)
	}
	if len(matData) == 0 {
		t.Fatal("matrix.json is empty")
	}
	// Compact JSON should not contain newlines.
	if slices.Contains(matData, '\n') {
		t.Error("matrix.json should be compact (no newlines)")
	}

	var gotMatrix MatrixOutput
	if err := json.Unmarshal(matData, &gotMatrix); err != nil {
		t.Fatalf("parsing matrix.json: %v", err)
	}
	if len(gotMatrix.Include) != 1 {
		t.Errorf("expected 1 matrix entry, got %d", len(gotMatrix.Include))
	}
}

func TestBuildPatchResults(t *testing.T) {
	dir := t.TempDir()

	// Set up a trivy report.
	reportsDir := filepath.Join(dir, "reports")
	if err := os.MkdirAll(reportsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	reportData := []byte(`{"Results":[{"Vulnerabilities":[{"FixedVersion":"1.0"}]}]}`)
	reportName := sanitize("quay.io/prometheus/prometheus:v3.9.1") + ".json"
	if err := os.WriteFile(filepath.Join(reportsDir, reportName), reportData, 0o644); err != nil {
		t.Fatal(err)
	}

	images := []ImageDiscovery{
		{Registry: "quay.io", Repository: "prometheus/prometheus", Tag: "v3.9.1", Path: "server.image"},
		{Registry: "docker.io", Repository: "grafana/grafana", Tag: "12.3.3", Path: "grafana.image"},
	}

	resultMap := map[string]*SinglePatchResult{
		"quay.io/prometheus/prometheus:v3.9.1": {
			ImageRef:          "quay.io/prometheus/prometheus:v3.9.1",
			PatchedRegistry:   testRegistry,
			PatchedRepository: "prometheus/prometheus",
			PatchedTag:        "v3.9.1-patched",
			VulnCount:         5,
		},
		"docker.io/grafana/grafana:12.3.3": {
			ImageRef:   "docker.io/grafana/grafana:12.3.3",
			Skipped:    true,
			SkipReason: "no fixable vulnerabilities",
		},
	}

	results := buildPatchResults(images, resultMap, reportsDir)

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	// First result: patched.
	r0 := results[0]
	if r0.Patched.Registry != testRegistry {
		t.Errorf("expected patched registry %q, got %q", testRegistry, r0.Patched.Registry)
	}
	if r0.Patched.Tag != "v3.9.1-patched" {
		t.Errorf("expected patched tag 'v3.9.1-patched', got %q", r0.Patched.Tag)
	}
	if r0.VulnCount != 5 {
		t.Errorf("expected vuln count 5, got %d", r0.VulnCount)
	}
	if r0.ReportPath == "" {
		t.Error("expected report path to be set")
	}
	if r0.Original.Path != "server.image" {
		t.Errorf("expected original path 'server.image', got %q", r0.Original.Path)
	}

	// Second result: skipped.
	r1 := results[1]
	if !r1.Skipped {
		t.Error("expected second result to be skipped")
	}
	if r1.SkipReason != "no fixable vulnerabilities" {
		t.Errorf("expected skip reason, got %q", r1.SkipReason)
	}
}

func TestLoadResults(t *testing.T) {
	dir := t.TempDir()

	// Write two result files.
	r1 := SinglePatchResult{
		ImageRef:          "quay.io/prom/prom:v1",
		PatchedRegistry:   "ghcr.io/test",
		PatchedRepository: "prom/prom",
		PatchedTag:        "v1-patched",
		VulnCount:         3,
	}
	r2 := SinglePatchResult{
		ImageRef:   "docker.io/nginx:1.25",
		Skipped:    true,
		SkipReason: "no fixable vulnerabilities",
	}

	for _, r := range []SinglePatchResult{r1, r2} {
		data, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, sanitize(r.ImageRef)+".json")
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Also write a non-JSON file that should be ignored.
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("ignore"), 0o644); err != nil {
		t.Fatal(err)
	}

	results, err := loadResults(dir)
	if err != nil {
		t.Fatalf("loadResults() error: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	if r, ok := results["quay.io/prom/prom:v1"]; !ok {
		t.Error("missing result for quay.io/prom/prom:v1")
	} else if r.VulnCount != 3 {
		t.Errorf("expected vuln count 3, got %d", r.VulnCount)
	}

	if r, ok := results["docker.io/nginx:1.25"]; !ok {
		t.Error("missing result for docker.io/nginx:1.25")
	} else if !r.Skipped {
		t.Error("expected nginx to be skipped")
	}
}

func TestLoadResultsEmptyDir(t *testing.T) {
	dir := t.TempDir()
	results, err := loadResults(dir)
	if err != nil {
		t.Fatalf("loadResults() error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestLoadResultsNonExistentDir(t *testing.T) {
	results, err := loadResults("/nonexistent/path")
	if err != nil {
		t.Fatalf("loadResults() should not error for missing dir: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestAssembleResults(t *testing.T) {
	dir := t.TempDir()

	// Write manifest.
	manifest := DiscoveryManifest{
		Charts: []ChartDiscovery{
			{
				Name:       "myapp",
				Version:    "1.0.0",
				Repository: "oci://ghcr.io/charts",
				Images: []ImageDiscovery{
					{Registry: "docker.io", Repository: "library/nginx", Tag: "1.25", Path: "image"},
				},
			},
		},
	}
	manifestData, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(manifestPath, manifestData, 0o644); err != nil {
		t.Fatal(err)
	}

	// Write result.
	resultsDir := filepath.Join(dir, "results")
	if err := os.MkdirAll(resultsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	result := SinglePatchResult{
		ImageRef:          "docker.io/library/nginx:1.25",
		PatchedRegistry:   "ghcr.io/test",
		PatchedRepository: "library/nginx",
		PatchedTag:        "1.25-patched",
		VulnCount:         2,
		Changed:           true, // Image was patched (changed)
	}
	rData, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(resultsDir, sanitize("docker.io/library/nginx:1.25")+".json"), rData, 0o644); err != nil {
		t.Fatal(err)
	}

	// Write a trivy report.
	reportsDir := filepath.Join(dir, "reports")
	if err := os.MkdirAll(reportsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	reportData := []byte(`{"Results":[{"Vulnerabilities":[{"FixedVersion":"1.0","VulnerabilityID":"CVE-2024-0001"}]}]}`)
	if err := os.WriteFile(filepath.Join(reportsDir, sanitize("docker.io/library/nginx:1.25")+".json"), reportData, 0o644); err != nil {
		t.Fatal(err)
	}

	// Run assemble (without publishing).
	outputDir := filepath.Join(dir, "charts")
	if err := AssembleResults(manifestPath, resultsDir, reportsDir, outputDir, "", false); err != nil {
		t.Fatalf("AssembleResults() error: %v", err)
	}

	// Verify wrapper chart was created.
	chartYaml := filepath.Join(outputDir, "myapp", "Chart.yaml")
	if _, err := os.Stat(chartYaml); err != nil {
		t.Errorf("wrapper Chart.yaml not created: %v", err)
	}

	valuesYaml := filepath.Join(outputDir, "myapp", "values.yaml")
	if _, err := os.Stat(valuesYaml); err != nil {
		t.Errorf("wrapper values.yaml not created: %v", err)
	}

	// Verify SBOM was generated.
	sbomPath := filepath.Join(outputDir, "myapp", "sbom.cdx.json")
	if _, err := os.Stat(sbomPath); err != nil {
		t.Errorf("SBOM should be created: %v", err)
	}

	// Verify vuln predicate was generated.
	vulnPath := filepath.Join(outputDir, "myapp", "vuln-predicate.json")
	if _, err := os.Stat(vulnPath); err != nil {
		t.Errorf("vuln predicate should be created: %v", err)
	}

	// Verify published-charts.json was created.
	publishedPath := filepath.Join(outputDir, "published-charts.json")
	if _, err := os.Stat(publishedPath); err != nil {
		t.Errorf("published-charts.json should be created: %v", err)
	}

	// Reports are now attached as in-toto attestations on each image,
	// NOT bundled in the chart package. Verify reports/ is NOT created.
	copiedReport := filepath.Join(outputDir, "myapp", "reports")
	if _, err := os.Stat(copiedReport); !os.IsNotExist(err) {
		t.Errorf("reports/ should not exist in wrapper chart (reports are in-toto attestations now)")
	}
}

func TestBuildPatchResultsMissingResult(t *testing.T) {
	images := []ImageDiscovery{
		{Registry: "docker.io", Repository: "library/nginx", Tag: "1.25", Path: "image"},
	}

	// Empty result map — no patch result for this image.
	resultMap := map[string]*SinglePatchResult{}
	reportsDir := t.TempDir()

	results := buildPatchResults(images, resultMap, reportsDir)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Skipped {
		t.Error("expected missing result to be marked as Skipped")
	}
	if results[0].SkipReason != "no patch result for image" {
		t.Errorf("unexpected skip reason: %q", results[0].SkipReason)
	}
	if results[0].Patched.Repository != "" {
		t.Error("expected empty Patched image for missing result")
	}
}

func TestImageDiscoveryReference(t *testing.T) {
	d := ImageDiscovery{Registry: "quay.io", Repository: "prom/prom", Tag: "v1"}
	if got := d.reference(); got != "quay.io/prom/prom:v1" {
		t.Errorf("reference() = %q, want %q", got, "quay.io/prom/prom:v1")
	}

	d2 := ImageDiscovery{Repository: "nginx", Tag: "latest"}
	if got := d2.reference(); got != "nginx:latest" {
		t.Errorf("reference() = %q, want %q", got, "nginx:latest")
	}
}

func TestChangedFieldInSinglePatchResult(t *testing.T) {
	tests := []struct {
		name        string
		result      SinglePatchResult
		wantChanged bool
	}{
		{
			name: "successfully patched image",
			result: SinglePatchResult{
				ImageRef:          "nginx:1.25",
				PatchedRegistry:   "ghcr.io/test",
				PatchedRepository: "nginx",
				PatchedTag:        "1.25-patched",
				Skipped:           false,
				Error:             "",
				Changed:           true,
			},
			wantChanged: true,
		},
		{
			name: "image already up to date",
			result: SinglePatchResult{
				ImageRef:   "nginx:1.25",
				Skipped:    true,
				SkipReason: "patched image up to date",
				Error:      "",
				Changed:    false,
			},
			wantChanged: false,
		},
		{
			name: "image mirrored (no fixable vulns)",
			result: SinglePatchResult{
				ImageRef:   "nginx:1.25",
				Skipped:    true,
				SkipReason: "no fixable vulnerabilities",
				Error:      "",
				Changed:    true,
			},
			wantChanged: true,
		},
		{
			name: "image patch failed",
			result: SinglePatchResult{
				ImageRef: "nginx:1.25",
				Error:    "failed to push",
				Changed:  false,
			},
			wantChanged: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.result.Changed != tt.wantChanged {
				t.Errorf("Changed = %v, want %v", tt.result.Changed, tt.wantChanged)
			}
		})
	}
}

func TestAssembleResultsSkipsUnchangedCharts(t *testing.T) {
	dir := t.TempDir()

	// Write manifest with one chart.
	manifest := DiscoveryManifest{
		Charts: []ChartDiscovery{
			{
				Name:       "myapp",
				Version:    "1.0.0",
				Repository: "oci://ghcr.io/charts",
				Images: []ImageDiscovery{
					{Registry: "docker.io", Repository: "library/nginx", Tag: "1.25", Path: "image"},
				},
			},
		},
	}
	manifestData, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(manifestPath, manifestData, 0o644); err != nil {
		t.Fatal(err)
	}

	// Write result with Changed=false (already up to date).
	resultsDir := filepath.Join(dir, "results")
	if err := os.MkdirAll(resultsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	result := SinglePatchResult{
		ImageRef:   "docker.io/library/nginx:1.25",
		Skipped:    true,
		SkipReason: "patched image up to date",
		Changed:    false,
	}
	rData, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(resultsDir, sanitize("docker.io/library/nginx:1.25")+".json"), rData, 0o644); err != nil {
		t.Fatal(err)
	}

	reportsDir := filepath.Join(dir, "reports")
	if err := os.MkdirAll(reportsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Run assemble.
	outputDir := filepath.Join(dir, "charts")
	if err := AssembleResults(manifestPath, resultsDir, reportsDir, outputDir, "", false); err != nil {
		t.Fatalf("AssembleResults() error: %v", err)
	}

	// Verify wrapper chart was NOT created (skipped due to no changes).
	chartYaml := filepath.Join(outputDir, "myapp", "Chart.yaml")
	if _, err := os.Stat(chartYaml); !os.IsNotExist(err) {
		t.Errorf("wrapper Chart.yaml should not be created when no images changed")
	}

	// Verify published-charts.json was not created or is empty.
	publishedPath := filepath.Join(outputDir, "published-charts.json")
	if data, err := os.ReadFile(publishedPath); err == nil {
		var charts []PublishedChart
		if err := json.Unmarshal(data, &charts); err == nil && len(charts) > 0 {
			t.Errorf("published-charts.json should be empty when no charts published, got %d charts", len(charts))
		}
	}
}

func TestAssembleResultsProcessesChangedCharts(t *testing.T) {
	dir := t.TempDir()

	// Write manifest with two charts.
	manifest := DiscoveryManifest{
		Charts: []ChartDiscovery{
			{
				Name:       "unchanged-app",
				Version:    "1.0.0",
				Repository: "oci://ghcr.io/charts",
				Images: []ImageDiscovery{
					{Registry: "docker.io", Repository: "library/nginx", Tag: "1.25", Path: "image"},
				},
			},
			{
				Name:       "changed-app",
				Version:    "2.0.0",
				Repository: "oci://ghcr.io/charts",
				Images: []ImageDiscovery{
					{Registry: "docker.io", Repository: "library/redis", Tag: "7.0", Path: "image"},
				},
			},
		},
	}
	manifestData, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(manifestPath, manifestData, 0o644); err != nil {
		t.Fatal(err)
	}

	// Write results: first unchanged, second changed.
	resultsDir := filepath.Join(dir, "results")
	if err := os.MkdirAll(resultsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	result1 := SinglePatchResult{
		ImageRef:   "docker.io/library/nginx:1.25",
		Skipped:    true,
		SkipReason: "patched image up to date",
		Changed:    false,
	}
	rData1, err := json.Marshal(result1)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(resultsDir, sanitize("docker.io/library/nginx:1.25")+".json"), rData1, 0o644); err != nil {
		t.Fatal(err)
	}

	result2 := SinglePatchResult{
		ImageRef:          "docker.io/library/redis:7.0",
		PatchedRegistry:   "ghcr.io/test",
		PatchedRepository: "library/redis",
		PatchedTag:        "7.0-patched",
		VulnCount:         3,
		Changed:           true,
	}
	rData2, err := json.Marshal(result2)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(resultsDir, sanitize("docker.io/library/redis:7.0")+".json"), rData2, 0o644); err != nil {
		t.Fatal(err)
	}

	// Write trivy report for the changed image.
	reportsDir := filepath.Join(dir, "reports")
	if err := os.MkdirAll(reportsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	reportData := []byte(`{"Results":[{"Vulnerabilities":[{"FixedVersion":"1.0","VulnerabilityID":"CVE-2024-0001"}]}]}`)
	if err := os.WriteFile(filepath.Join(reportsDir, sanitize("docker.io/library/redis:7.0")+".json"), reportData, 0o644); err != nil {
		t.Fatal(err)
	}

	// Run assemble.
	outputDir := filepath.Join(dir, "charts")
	if err := AssembleResults(manifestPath, resultsDir, reportsDir, outputDir, "ghcr.io/test", false); err != nil {
		t.Fatalf("AssembleResults() error: %v", err)
	}

	// Verify unchanged-app was NOT created.
	unchangedChart := filepath.Join(outputDir, "unchanged-app", "Chart.yaml")
	if _, err := os.Stat(unchangedChart); !os.IsNotExist(err) {
		t.Errorf("unchanged-app Chart.yaml should not be created")
	}

	// Verify changed-app WAS created.
	changedChart := filepath.Join(outputDir, "changed-app", "Chart.yaml")
	if _, err := os.Stat(changedChart); err != nil {
		t.Errorf("changed-app Chart.yaml should be created: %v", err)
	}

	changedValues := filepath.Join(outputDir, "changed-app", "values.yaml")
	if _, err := os.Stat(changedValues); err != nil {
		t.Errorf("changed-app values.yaml should be created: %v", err)
	}

	// Verify SBOM was generated.
	sbomPath := filepath.Join(outputDir, "changed-app", "sbom.cdx.json")
	if _, err := os.Stat(sbomPath); err != nil {
		t.Errorf("SBOM should be created: %v", err)
	}

	// Verify vuln predicate was generated.
	vulnPath := filepath.Join(outputDir, "changed-app", "vuln-predicate.json")
	if _, err := os.Stat(vulnPath); err != nil {
		t.Errorf("vuln predicate should be created: %v", err)
	}

	// Verify published-charts.json contains only the changed chart.
	publishedPath := filepath.Join(outputDir, "published-charts.json")
	data, err := os.ReadFile(publishedPath)
	if err != nil {
		t.Fatalf("published-charts.json should be created: %v", err)
	}

	var charts []PublishedChart
	if err := json.Unmarshal(data, &charts); err != nil {
		t.Fatalf("failed to parse published-charts.json: %v", err)
	}

	if len(charts) != 1 {
		t.Fatalf("expected 1 published chart, got %d", len(charts))
	}

	if charts[0].Name != "changed-app" {
		t.Errorf("expected published chart 'changed-app', got %q", charts[0].Name)
	}

	if charts[0].Version != "2.0.0-0" {
		t.Errorf("expected version '2.0.0-0', got %q", charts[0].Version)
	}

	if len(charts[0].Images) != 1 {
		t.Errorf("expected 1 image in published chart, got %d", len(charts[0].Images))
	}
}
