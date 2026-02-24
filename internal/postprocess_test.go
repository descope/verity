package internal

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

const (
	testChartPrometheus  = "prometheus"
	testChartStandalone  = "standalone"
	testRegistryDockerIO = "docker.io"
)

func TestPostProcessCopaResults(t *testing.T) {
	tmpDir := t.TempDir()

	// Create Copa output file
	copaOutputPath := filepath.Join(tmpDir, "copa-output.json")
	copaJSON := `{
  "results": [
    {
      "name": "nginx",
      "status": "Patched",
      "source_image": "docker.io/library/nginx:1.25.3",
      "patched_image": "ghcr.io/verity-org/library/nginx:1.25.3-patched",
      "details": "OK"
    },
    {
      "name": "prometheus",
      "status": "Skipped",
      "source_image": "quay.io/prometheus/prometheus:v3.9.1",
      "patched_image": "ghcr.io/verity-org/prometheus/prometheus:v3.9.1-patched",
      "details": "no fixable vulnerabilities"
    },
    {
      "name": "grafana",
      "status": "Failed",
      "source_image": "docker.io/grafana/grafana:12.3.3",
      "patched_image": "",
      "details": "patch failed: build error"
    }
  ]
}`
	if err := os.WriteFile(copaOutputPath, []byte(copaJSON), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	// Create chart-image-map file
	chartMapPath := filepath.Join(tmpDir, "chart-image-map.yaml")
	chartMapYAML := `charts:
  - name: prometheus
    version: "28.9.1"
    repository: "oci://ghcr.io/prometheus-community/charts"
    images:
      - quay.io/prometheus/prometheus
  - name: standalone
    version: "0.0.0"
    repository: "file://./charts/standalone"
    images:
      - docker.io/library/nginx
      - docker.io/grafana/grafana
`
	if err := os.WriteFile(chartMapPath, []byte(chartMapYAML), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	// Run post-process
	opts := PostProcessOptions{
		CopaOutputPath:   copaOutputPath,
		ChartMapPath:     chartMapPath,
		RegistryPrefix:   "ghcr.io/verity-org",
		OutputDir:        filepath.Join(tmpDir, "output"),
		SkipDigestLookup: true, // Skip registry lookups in test
	}

	result, err := PostProcessCopaResults(opts)
	if err != nil {
		t.Fatalf("PostProcessCopaResults() error = %v", err)
	}

	// Verify counts
	if result.PatchedCount != 1 {
		t.Errorf("PatchedCount = %d, want 1", result.PatchedCount)
	}
	if result.SkippedCount != 1 {
		t.Errorf("SkippedCount = %d, want 1", result.SkippedCount)
	}
	if result.FailedCount != 1 {
		t.Errorf("FailedCount = %d, want 1", result.FailedCount)
	}
	if result.ChartCount != 2 {
		t.Errorf("ChartCount = %d, want 2", result.ChartCount)
	}

	// Verify matrix.json was created
	if _, err := os.Stat(result.MatrixPath); os.IsNotExist(err) {
		t.Errorf("matrix.json was not created")
	}

	// Read and verify matrix content
	matrixData, err := os.ReadFile(result.MatrixPath)
	if err != nil {
		t.Fatalf("ReadFile(matrix.json) error = %v", err)
	}
	var matrix MatrixOutput
	if err := json.Unmarshal(matrixData, &matrix); err != nil {
		t.Fatalf("Unmarshal(matrix) error = %v", err)
	}
	// Only patched images should be in matrix
	if len(matrix.Include) != 1 {
		t.Errorf("matrix.Include length = %d, want 1 (only patched images)", len(matrix.Include))
	}

	// Verify manifest.json was created
	if _, err := os.Stat(result.ManifestPath); os.IsNotExist(err) {
		t.Errorf("manifest.json was not created")
	}

	// Read and verify manifest content
	manifestData, err := os.ReadFile(result.ManifestPath)
	if err != nil {
		t.Fatalf("ReadFile(manifest.json) error = %v", err)
	}
	var manifest DiscoveryManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatalf("Unmarshal(manifest) error = %v", err)
	}
	if len(manifest.Charts) != 2 {
		t.Errorf("manifest.Charts length = %d, want 2", len(manifest.Charts))
	}
	if manifest.Charts[0].Name != testChartPrometheus {
		t.Errorf("manifest.Charts[0].Name = %v, want prometheus", manifest.Charts[0].Name)
	}
	if manifest.Charts[1].Name != testChartStandalone {
		t.Errorf("manifest.Charts[1].Name = %v, want standalone", manifest.Charts[1].Name)
	}

	// Verify result files were created
	resultsDir := filepath.Join(opts.OutputDir, "results")
	resultFiles, err := os.ReadDir(resultsDir)
	if err != nil {
		t.Fatalf("ReadDir(results) error = %v", err)
	}
	// Should have 3 result files (one per Copa result)
	if len(resultFiles) != 3 {
		t.Errorf("result files count = %d, want 3", len(resultFiles))
	}
}

func TestSanitizeImageName(t *testing.T) {
	tests := []struct {
		name string
		ref  string
		want string
	}{
		{
			name: "docker hub image",
			ref:  "docker.io/library/nginx:1.25.3",
			want: "docker_io_library_nginx_1_25_3",
		},
		{
			name: "quay.io image",
			ref:  "quay.io/prometheus/prometheus:v3.9.1",
			want: "quay_io_prometheus_prometheus_v3_9_1",
		},
		{
			name: "ghcr image with digest",
			ref:  "ghcr.io/verity-org/nginx@sha256:abc123",
			want: "ghcr_io_verity-org_nginx_sha256_abc123",
		},
		{
			name: "simple reference",
			ref:  "nginx:latest",
			want: "nginx_latest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sanitizeImageName(tt.ref); got != tt.want {
				t.Errorf("sanitizeImageName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGenerateManifest(t *testing.T) {
	chartMap := &ChartImageMap{
		Charts: []ChartImageEntry{
			{
				Name:       testChartPrometheus,
				Version:    "28.9.1",
				Repository: "oci://ghcr.io/prometheus-community/charts",
				Images: []string{
					"quay.io/prometheus/prometheus",
					"quay.io/prometheus/alertmanager",
				},
			},
			{
				Name:       testChartStandalone,
				Version:    "0.0.0",
				Repository: "file://./charts/standalone",
				Images: []string{
					"docker.io/library/nginx",
					"grafana/grafana", // test without registry prefix
				},
			},
		},
	}

	resultMap := map[string]*CopaOutputResult{
		"quay.io/prometheus/prometheus": {
			Name:         "prometheus",
			Status:       "Patched",
			SourceImage:  "quay.io/prometheus/prometheus:v3.9.1",
			PatchedImage: "ghcr.io/verity-org/prometheus/prometheus:v3.9.1-patched",
		},
		"docker.io/library/nginx": {
			Name:         "nginx",
			Status:       "Skipped",
			SourceImage:  "docker.io/library/nginx:1.25.3",
			PatchedImage: "ghcr.io/verity-org/library/nginx:1.25.3-patched",
		},
	}

	manifest := generateManifest(chartMap, resultMap)

	if len(manifest.Charts) != 2 {
		t.Errorf("manifest.Charts length = %d, want 2", len(manifest.Charts))
	}

	if manifest.Charts[0].Name != testChartPrometheus {
		t.Errorf("manifest.Charts[0].Name = %v, want prometheus", manifest.Charts[0].Name)
	}
	if len(manifest.Charts[0].Images) != 2 {
		t.Errorf("manifest.Charts[0].Images length = %d, want 2", len(manifest.Charts[0].Images))
	}

	if manifest.Charts[1].Name != testChartStandalone {
		t.Errorf("manifest.Charts[1].Name = %v, want standalone", manifest.Charts[1].Name)
	}
	if len(manifest.Charts[1].Images) != 2 {
		t.Errorf("manifest.Charts[1].Images length = %d, want 2", len(manifest.Charts[1].Images))
	}

	// Check that grafana was normalized to include docker.io
	grafanaImg := manifest.Charts[1].Images[1]
	if grafanaImg.Registry != testRegistryDockerIO {
		t.Errorf("grafana image registry = %v, want docker.io", grafanaImg.Registry)
	}
	if grafanaImg.Repository != "grafana/grafana" {
		t.Errorf("grafana image repository = %v, want grafana/grafana", grafanaImg.Repository)
	}

	// Check flat images list
	totalImages := len(manifest.Charts[0].Images) + len(manifest.Charts[1].Images)
	if len(manifest.Images) != totalImages {
		t.Errorf("manifest.Images length = %d, want %d", len(manifest.Images), totalImages)
	}
}
