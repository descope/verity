package discovery

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/verity-org/verity/internal/config"
)

const testNginxName = "nginx"

func TestNameFromRef(t *testing.T) {
	tests := []struct {
		name string
		ref  string
		want string
	}{
		{
			name: "org equals name: deduplicated",
			ref:  "quay.io/prometheus/prometheus:v3.2.1",
			want: "prometheus",
		},
		{
			name: "org equals name with digest: deduplicated",
			ref:  "quay.io/prometheus/prometheus@sha256:abc123",
			want: "prometheus",
		},
		{
			name: "simple image with no org",
			ref:  "nginx:1.25",
			want: testNginxName,
		},
		{
			name: "org differs from name: org-name joined",
			ref:  "ghcr.io/kiwigrid/k8s-sidecar:1.28.0",
			want: "kiwigrid-k8s-sidecar",
		},
		{
			name: "collision safety: different registry same basename",
			ref:  "quay.io/some-org/nginx:1.29",
			want: "some-org-nginx",
		},
		{
			name: "registry.k8s.io with repeated component",
			ref:  "registry.k8s.io/kube-state-metrics/kube-state-metrics:v2.10.0",
			want: "kube-state-metrics",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := nameFromRef(tc.ref)
			if got != tc.want {
				t.Errorf("nameFromRef(%q) = %q, want %q", tc.ref, got, tc.want)
			}
		})
	}
}

func TestJoinPlatforms(t *testing.T) {
	tests := []struct {
		name      string
		platforms []string
		want      string
	}{
		{
			name:      "nil defaults to amd64+arm64",
			platforms: nil,
			want:      "linux/amd64,linux/arm64",
		},
		{
			name:      "empty defaults to amd64+arm64",
			platforms: []string{},
			want:      "linux/amd64,linux/arm64",
		},
		{
			name:      "explicit platforms joined",
			platforms: []string{"linux/amd64", "linux/arm64"},
			want:      "linux/amd64,linux/arm64",
		},
		{
			name:      "single platform",
			platforms: []string{"linux/amd64"},
			want:      "linux/amd64",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := joinPlatforms(tc.platforms)
			if got != tc.want {
				t.Errorf("joinPlatforms(%v) = %q, want %q", tc.platforms, got, tc.want)
			}
		})
	}
}

func TestDiscoverStandaloneImage_List(t *testing.T) {
	spec := &config.ImageSpec{
		Name:  testNginxName,
		Image: "mirror.gcr.io/library/nginx",
		Tags: config.TagStrategy{
			Strategy: "list",
			List:     []string{"1.25.3", "1.26.0"},
		},
		Platforms: []string{"linux/amd64", "linux/arm64"},
	}

	got, err := discoverStandaloneImage(spec, "ghcr.io/verity-org")
	if err != nil {
		t.Fatalf("discoverStandaloneImage() error = %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("discoverStandaloneImage() returned %d images, want 2", len(got))
	}

	for i, img := range got {
		if img.Name != testNginxName {
			t.Errorf("[%d] Name = %q, want %q", i, img.Name, testNginxName)
		}
		if img.TargetRegistry != "ghcr.io/verity-org" {
			t.Errorf("[%d] TargetRegistry = %q, want %q", i, img.TargetRegistry, "ghcr.io/verity-org")
		}
		if img.Platforms != "linux/amd64,linux/arm64" {
			t.Errorf("[%d] Platforms = %q, want %q", i, img.Platforms, "linux/amd64,linux/arm64")
		}
	}

	if got[0].Source != "mirror.gcr.io/library/nginx:1.25.3" {
		t.Errorf("[0] Source = %q, want %q", got[0].Source, "mirror.gcr.io/library/nginx:1.25.3")
	}
	if got[1].Source != "mirror.gcr.io/library/nginx:1.26.0" {
		t.Errorf("[1] Source = %q, want %q", got[1].Source, "mirror.gcr.io/library/nginx:1.26.0")
	}
}

func TestDiscoverStandaloneImage_PerImageRegistryOverride(t *testing.T) {
	spec := &config.ImageSpec{
		Name:  testNginxName,
		Image: "mirror.gcr.io/library/nginx",
		Tags: config.TagStrategy{
			Strategy: "list",
			List:     []string{"1.25.3"},
		},
		Target: config.TargetSpec{Registry: "ghcr.io/custom-org"},
	}

	got, err := discoverStandaloneImage(spec, "ghcr.io/verity-org")
	if err != nil {
		t.Fatalf("discoverStandaloneImage() error = %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("discoverStandaloneImage() returned %d images, want 1", len(got))
	}

	// Per-image registry should override the global registry
	if got[0].TargetRegistry != "ghcr.io/custom-org" {
		t.Errorf("TargetRegistry = %q, want %q", got[0].TargetRegistry, "ghcr.io/custom-org")
	}
}

func TestLoadConfig(t *testing.T) {
	yaml := `
apiVersion: copa.sh/v1alpha1
kind: PatchConfig
target:
  registry: ghcr.io/test-org
images:
  - name: nginx
    image: mirror.gcr.io/library/nginx
    platforms: [linux/amd64, linux/arm64]
    tags:
      strategy: list
      list: ["1.25.3"]
`
	dir := t.TempDir()
	path := filepath.Join(dir, "copa-config.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if cfg.Target.Registry != "ghcr.io/test-org" {
		t.Errorf("Target.Registry = %q, want %q", cfg.Target.Registry, "ghcr.io/test-org")
	}
	if len(cfg.Images) != 1 || cfg.Images[0].Name != testNginxName {
		t.Errorf("Images = %v, want [{nginx ...}]", cfg.Images)
	}
}

func TestLoadChartsFile(t *testing.T) {
	yaml := `
apiVersion: v2
name: verity
dependencies:
  - name: prometheus
    version: "28.9.1"
    repository: "oci://ghcr.io/prometheus-community/charts"
  - name: postgres-operator
    version: "1.15.1"
    repository: "https://opensource.zalando.com/postgres-operator/charts/postgres-operator"
`
	dir := t.TempDir()
	path := filepath.Join(dir, "Chart.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	charts, err := LoadChartsFile(path)
	if err != nil {
		t.Fatalf("LoadChartsFile() error = %v", err)
	}

	if len(charts) != 2 {
		t.Fatalf("LoadChartsFile() returned %d charts, want 2", len(charts))
	}
	if charts[0].Name != "prometheus" || charts[0].Version != "28.9.1" {
		t.Errorf("charts[0] = %+v, want {prometheus 28.9.1 ...}", charts[0])
	}
	if charts[1].Name != "postgres-operator" {
		t.Errorf("charts[1].Name = %q, want postgres-operator", charts[1].Name)
	}
}

func TestLoadVerityConfig(t *testing.T) {
	yaml := `
overrides:
  "timberio/vector":
    from: "distroless-libc"
    to: "debian"
`
	dir := t.TempDir()
	path := filepath.Join(dir, "verity.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	vc, err := LoadVerityConfig(path)
	if err != nil {
		t.Fatalf("LoadVerityConfig() error = %v", err)
	}

	override, ok := vc.Overrides["timberio/vector"]
	if !ok {
		t.Fatal("LoadVerityConfig() missing expected override key")
	}
	if override.From != "distroless-libc" || override.To != "debian" {
		t.Errorf("override = %+v, want {From: distroless-libc, To: debian}", override)
	}
}

func TestLoadVerityConfig_Missing(t *testing.T) {
	vc, err := LoadVerityConfig("/nonexistent/verity.yaml")
	if err != nil {
		t.Fatalf("LoadVerityConfig() expected nil error for missing file, got %v", err)
	}
	if vc == nil || vc.Overrides != nil {
		t.Errorf("LoadVerityConfig() expected empty config for missing file, got %+v", vc)
	}
}

func TestLoadChartsFile_Missing(t *testing.T) {
	charts, err := LoadChartsFile("/nonexistent/Chart.yaml")
	if err != nil {
		t.Fatalf("LoadChartsFile() expected nil error for missing file, got %v", err)
	}
	if charts != nil {
		t.Errorf("LoadChartsFile() expected nil slice for missing file, got %v", charts)
	}
}

func TestLoadConfig_MissingFile(t *testing.T) {
	_, err := LoadConfig("/nonexistent/path/copa-config.yaml")
	if err == nil {
		t.Fatal("LoadConfig() expected error for missing file, got nil")
	}
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "copa-config.yaml")
	if err := os.WriteFile(path, []byte("{ invalid yaml: ["), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("LoadConfig() expected error for invalid YAML, got nil")
	}
}

func TestDiscover_StandaloneOnly(t *testing.T) {
	cfg := &config.CopaConfig{
		Target: config.TargetSpec{Registry: "ghcr.io/verity-org"},
		Images: []config.ImageSpec{
			{
				Name:  testNginxName,
				Image: "mirror.gcr.io/library/nginx",
				Tags:  config.TagStrategy{Strategy: "list", List: []string{"1.25.3", "1.26.0"}},
			},
			{
				Name:  "redis",
				Image: "quay.io/opstree/redis",
				Tags:  config.TagStrategy{Strategy: "list", List: []string{"v7.0.5"}},
			},
		},
	}

	got, err := Discover(cfg, "", nil)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}

	if len(got) != 3 {
		t.Errorf("Discover() returned %d images, want 3", len(got))
	}

	// Verify target registry from config
	for _, img := range got {
		if img.TargetRegistry != "ghcr.io/verity-org" {
			t.Errorf("TargetRegistry = %q, want %q", img.TargetRegistry, "ghcr.io/verity-org")
		}
	}
}

func TestDiscover_TargetRegistryOverride(t *testing.T) {
	cfg := &config.CopaConfig{
		Target: config.TargetSpec{Registry: "ghcr.io/config-org"},
		Images: []config.ImageSpec{
			{
				Name:  testNginxName,
				Image: "mirror.gcr.io/library/nginx",
				Tags:  config.TagStrategy{Strategy: "list", List: []string{"1.25.3"}},
			},
		},
	}

	got, err := Discover(cfg, "ghcr.io/override-org", nil)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("Discover() returned %d images, want 1", len(got))
	}
	if got[0].TargetRegistry != "ghcr.io/override-org" {
		t.Errorf("TargetRegistry = %q, want %q", got[0].TargetRegistry, "ghcr.io/override-org")
	}
}

func TestDiscover_Deduplication(t *testing.T) {
	cfg := &config.CopaConfig{
		Target: config.TargetSpec{Registry: "ghcr.io/verity-org"},
		Images: []config.ImageSpec{
			{
				Name:  testNginxName,
				Image: "mirror.gcr.io/library/nginx",
				Tags:  config.TagStrategy{Strategy: "list", List: []string{"1.25.3"}},
			},
			// Duplicate entry
			{
				Name:  testNginxName,
				Image: "mirror.gcr.io/library/nginx",
				Tags:  config.TagStrategy{Strategy: "list", List: []string{"1.25.3"}},
			},
		},
	}

	got, err := Discover(cfg, "", nil)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}

	if len(got) != 1 {
		t.Errorf("Discover() returned %d images, want 1 (deduplication)", len(got))
	}
}

func TestDiscover_ChartErrorContinues(t *testing.T) {
	// A chart with an invalid helm repository should log a warning and not
	// block standalone image discovery (Discover returns partial results).
	cfg := &config.CopaConfig{
		Target: config.TargetSpec{Registry: "ghcr.io/verity-org"},
		Images: []config.ImageSpec{
			{
				Name:  testNginxName,
				Image: "mirror.gcr.io/library/nginx",
				Tags:  config.TagStrategy{Strategy: "list", List: []string{"1.25.3"}},
			},
		},
		Charts: []config.ChartSpec{
			// Bogus chart — helm will fail, should be skipped with a warning.
			{
				Name:       "nonexistent-chart",
				Version:    "0.0.1",
				Repository: "https://nonexistent.invalid/charts",
			},
		},
	}

	got, err := Discover(cfg, "", nil)
	if err != nil {
		t.Fatalf("Discover() error = %v (should be nil even with chart failures)", err)
	}

	// Standalone image still discovered despite chart error
	if len(got) != 1 || got[0].Name != testNginxName {
		t.Errorf("Discover() = %v, want [{nginx ...}]", got)
	}
}

func TestDiscover_InvalidImageWarningContinues(t *testing.T) {
	// An image with an invalid repo (non-existent registry) for pattern/latest strategy
	// should produce a warning and not block other image discovery.
	cfg := &config.CopaConfig{
		Target: config.TargetSpec{Registry: "ghcr.io/verity-org"},
		Images: []config.ImageSpec{
			{
				Name:  "bad-image",
				Image: "thisregistrydoesnotexist.invalid/library/bad",
				Tags:  config.TagStrategy{Strategy: "latest"},
			},
			{
				Name:  "good-image",
				Image: "mirror.gcr.io/library/nginx",
				Tags:  config.TagStrategy{Strategy: "list", List: []string{"1.25.3"}},
			},
		},
	}

	got, err := Discover(cfg, "", nil)
	if err != nil {
		t.Fatalf("Discover() error = %v (should be nil even with image failures)", err)
	}

	// Only the good image should be discovered
	if len(got) != 1 || got[0].Name != "good-image" {
		t.Errorf("Discover() = %v, want [{good-image ...}]", got)
	}
}
