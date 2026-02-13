package internal

import (
	"encoding/json"
	"os"
	"path/filepath"
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
		Standalone: []ImageDiscovery{
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
	for _, b := range matData {
		if b == '\n' {
			t.Error("matrix.json should be compact (no newlines)")
			break
		}
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
			PatchedRegistry:   "ghcr.io/descope",
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
	if r0.Patched.Registry != "ghcr.io/descope" {
		t.Errorf("expected patched registry 'ghcr.io/descope', got %q", r0.Patched.Registry)
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
		data, _ := json.Marshal(r)
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
	manifestData, _ := json.Marshal(manifest)
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
	}
	rData, _ := json.Marshal(result)
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

	// Run assemble.
	outputDir := filepath.Join(dir, "charts")
	if err := AssembleResults(manifestPath, resultsDir, reportsDir, outputDir, ""); err != nil {
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

	// Verify the trivy report was copied into the wrapper chart.
	copiedReport := filepath.Join(outputDir, "myapp", "reports", sanitize("docker.io/library/nginx:1.25")+".json")
	if _, err := os.Stat(copiedReport); err != nil {
		t.Errorf("trivy report not copied to wrapper chart: %v", err)
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
