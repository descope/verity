package discovery

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/verity-org/verity/internal/config"
)

const testNginxName = "nginx"

func TestRepoPath(t *testing.T) {
	tests := []struct {
		name string
		ref  string
		want string
	}{
		{
			name: "org equals name",
			ref:  "quay.io/prometheus/prometheus:v3.2.1",
			want: "prometheus/prometheus",
		},
		{
			name: "org equals name with digest",
			ref:  "quay.io/prometheus/prometheus@sha256:abc123",
			want: "prometheus/prometheus",
		},
		{
			name: "simple image with no org",
			ref:  "nginx:1.25",
			want: testNginxName,
		},
		{
			name: "org differs from name",
			ref:  "ghcr.io/kiwigrid/k8s-sidecar:1.28.0",
			want: "kiwigrid/k8s-sidecar",
		},
		{
			name: "different registry same basename",
			ref:  "quay.io/some-org/nginx:1.29",
			want: "some-org/nginx",
		},
		{
			name: "registry.k8s.io with repeated component",
			ref:  "registry.k8s.io/kube-state-metrics/kube-state-metrics:v2.10.0",
			want: "kube-state-metrics/kube-state-metrics",
		},
		{
			name: "mirror gcr org image",
			ref:  "mirror.gcr.io/valkey/valkey:9.0.3",
			want: "valkey/valkey",
		},
		{
			name: "gcr org image",
			ref:  "gcr.io/distroless/static:latest",
			want: "distroless/static",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := repoPath(tc.ref)
			if got != tc.want {
				t.Errorf("repoPath(%q) = %q, want %q", tc.ref, got, tc.want)
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

func TestDiscoverStandaloneImage_GoVcsUrl(t *testing.T) {
	spec := &config.ImageSpec{
		Name:           "rabbitmqoperator/cluster-operator",
		Image:          "ghcr.io/rabbitmq/cluster-operator",
		Tags:           config.TagStrategy{Strategy: "list", List: []string{"2.19.0"}},
		Platforms:      []string{"linux/amd64", "linux/arm64"},
		GoVcsURL:       "https://github.com/rabbitmq/cluster-operator",
		GoVcsTagPrefix: "v",
	}

	got, err := discoverStandaloneImage(spec, "ghcr.io/verity-org")
	if err != nil {
		t.Fatalf("discoverStandaloneImage() error = %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("discoverStandaloneImage() returned %d images, want 1", len(got))
	}

	wantVcsUrl := "https://github.com/rabbitmq/cluster-operator@v2.19.0"
	if got[0].GoVcsURL != wantVcsUrl {
		t.Errorf("GoVcsUrl = %q, want %q", got[0].GoVcsURL, wantVcsUrl)
	}
}

func TestDiscoverStandaloneImage_GoVcsUrlEmpty(t *testing.T) {
	spec := &config.ImageSpec{
		Name:  testNginxName,
		Image: "mirror.gcr.io/library/nginx",
		Tags:  config.TagStrategy{Strategy: "list", List: []string{"1.25.3"}},
	}

	got, err := discoverStandaloneImage(spec, "ghcr.io/verity-org")
	if err != nil {
		t.Fatalf("discoverStandaloneImage() error = %v", err)
	}

	if got[0].GoVcsURL != "" {
		t.Errorf("GoVcsUrl = %q, want empty for non-Go images", got[0].GoVcsURL)
	}
}

func TestDiscoverStandaloneImage_GoVcsUrlNoPrefix(t *testing.T) {
	// Images with goVcsUrl but no tag prefix should still construct URL correctly.
	spec := &config.ImageSpec{
		Name:     "zalando/postgres-operator",
		Image:    "ghcr.io/zalando/postgres-operator",
		Tags:     config.TagStrategy{Strategy: "list", List: []string{"v1.15.1"}},
		GoVcsURL: "https://github.com/zalando/postgres-operator",
	}

	got, err := discoverStandaloneImage(spec, "ghcr.io/verity-org")
	if err != nil {
		t.Fatalf("discoverStandaloneImage() error = %v", err)
	}

	wantVcsUrl := "https://github.com/zalando/postgres-operator@v1.15.1"
	if got[0].GoVcsURL != wantVcsUrl {
		t.Errorf("GoVcsUrl = %q, want %q", got[0].GoVcsURL, wantVcsUrl)
	}
}

func TestLoadConfig_GoVcsUrl(t *testing.T) {
	yaml := `
apiVersion: copa.sh/v1alpha1
kind: PatchConfig
target:
  registry: ghcr.io/test-org
images:
  - name: rabbitmqoperator/cluster-operator
    image: ghcr.io/rabbitmq/cluster-operator
    platforms: [linux/amd64, linux/arm64]
    tags:
      strategy: list
      list: ["2.19.0"]
    goVcsUrl: "https://github.com/rabbitmq/cluster-operator"
    goVcsTagPrefix: "v"
  - name: nginx
    image: mirror.gcr.io/library/nginx
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

	if len(cfg.Images) != 2 {
		t.Fatalf("LoadConfig() returned %d images, want 2", len(cfg.Images))
	}

	// First image should have GoVcsUrl parsed.
	if cfg.Images[0].GoVcsURL != "https://github.com/rabbitmq/cluster-operator" {
		t.Errorf("GoVcsUrl = %q, want %q", cfg.Images[0].GoVcsURL, "https://github.com/rabbitmq/cluster-operator")
	}
	if cfg.Images[0].GoVcsTagPrefix != "v" {
		t.Errorf("GoVcsTagPrefix = %q, want %q", cfg.Images[0].GoVcsTagPrefix, "v")
	}

	// Second image should have empty GoVcsUrl.
	if cfg.Images[1].GoVcsURL != "" {
		t.Errorf("GoVcsUrl = %q, want empty", cfg.Images[1].GoVcsURL)
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

func TestLoadVerityConfig_ChartImageOverrides(t *testing.T) {
	yaml := `
chartImageOverrides:
  strimzi-kafka-operator:
    - source: STRIMZI_DEFAULT_KAFKA_EXPORTER_IMAGE
      path: kafka.image
    - source: STRIMZI_KAFKA_IMAGES
      type: csv
      path: kafka.versions["{version}"]
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

	overrides := vc.ChartImageOverrides["strimzi-kafka-operator"]
	if len(overrides) != 2 {
		t.Fatalf("len(strimzi overrides) = %d, want 2", len(overrides))
	}
	if overrides[0].Source != "STRIMZI_DEFAULT_KAFKA_EXPORTER_IMAGE" || overrides[0].Path != "kafka.image" {
		t.Fatalf("overrides[0] = %+v, want single-source override", overrides[0])
	}
	if overrides[1].Source != "STRIMZI_KAFKA_IMAGES" || overrides[1].Type != "csv" || overrides[1].Path != "kafka.versions[\"{version}\"]" {
		t.Fatalf("overrides[1] = %+v, want csv override", overrides[1])
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
				Name:  "valkey",
				Image: "mirror.gcr.io/valkey/valkey",
				Tags:  config.TagStrategy{Strategy: "list", List: []string{"9.0.3"}},
			},
		},
	}

	got, err := Discover(cfg, "", nil, nil, nil)
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

	got, err := Discover(cfg, "ghcr.io/override-org", nil, nil, nil)
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

	got, err := Discover(cfg, "", nil, nil, nil)
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

	got, err := Discover(cfg, "", nil, nil, nil)
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

	got, err := Discover(cfg, "", nil, nil, nil)
	if err != nil {
		t.Fatalf("Discover() error = %v (should be nil even with image failures)", err)
	}

	// Only the good image should be discovered
	if len(got) != 1 || got[0].Name != "good-image" {
		t.Errorf("Discover() = %v, want [{good-image ...}]", got)
	}
}

func TestDiscover_ExcludeChartNames(t *testing.T) {
	// Stub a fake helm binary that outputs a minimal manifest with image fields.
	// This exercises the full chart-discovery → exclude-names path.
	binDir := t.TempDir()
	helmScript := filepath.Join(binDir, "helm")
	script := `#!/bin/sh
cat <<'YAML'
apiVersion: apps/v1
kind: Deployment
metadata:
  name: test
spec:
  template:
    spec:
      containers:
      - name: prometheus
        image: quay.io/prometheus/prometheus:v3.0.0
      - name: sidecar
        image: ghcr.io/kiwigrid/k8s-sidecar:1.28.0
      - name: rabbitmq
        image: docker.io/library/rabbitmq:4.2.3
YAML
`
	if err := os.WriteFile(helmScript, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))

	cfg := &config.CopaConfig{
		Target: config.TargetSpec{Registry: "ghcr.io/verity-org"},
		Images: []config.ImageSpec{
			{
				Name:  "prometheus",
				Image: "mirror.gcr.io/library/prometheus",
				Tags:  config.TagStrategy{Strategy: "list", List: []string{"v3.0.0"}},
			},
		},
		Charts: []config.ChartSpec{
			{Name: "test-chart", Version: "1.0.0", Repository: "oci://ghcr.io/test/charts"},
		},
	}

	// Exclude "prometheus" and "rabbitmq" — both should be filtered from chart images.
	exclude := map[string]struct{}{"prometheus": {}, "rabbitmq": {}}

	got, err := Discover(cfg, "", nil, exclude, nil)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}

	// Expect:
	// - standalone "prometheus" (NOT excluded — standalone images are never filtered)
	// Excluded:
	names := make([]string, 0, len(got))
	for _, img := range got {
		names = append(names, img.Name)
	}

	if len(got) != 2 {
		t.Fatalf("Discover() returned %d images %v, want 2 [prometheus, kiwigrid/k8s-sidecar]", len(got), names)
	}

	// Standalone prometheus must survive (standalone images are never excluded).
	if got[0].Name != "prometheus" || got[0].Source != "mirror.gcr.io/library/prometheus:v3.0.0" {
		t.Errorf("got[0] = %+v, want standalone prometheus", got[0])
	}
	if got[1].Name != "kiwigrid/k8s-sidecar" {
		t.Errorf("got[1].Name = %q, want kiwigrid/k8s-sidecar", got[1].Name)
	}
}

func TestDiscover_ExcludeNamesNil(t *testing.T) {
	// nil excludeNames should not affect anything.
	cfg := &config.CopaConfig{
		Target: config.TargetSpec{Registry: "ghcr.io/verity-org"},
		Images: []config.ImageSpec{
			{
				Name:  testNginxName,
				Image: "mirror.gcr.io/library/nginx",
				Tags:  config.TagStrategy{Strategy: "list", List: []string{"1.25.3"}},
			},
		},
	}

	got, err := Discover(cfg, "", nil, nil, nil)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}

	if len(got) != 1 || got[0].Name != testNginxName {
		t.Errorf("Discover() = %v, want [{nginx ...}]", got)
	}
}

func TestNameBasename(t *testing.T) {
	tests := []struct {
		ref  string
		want string
	}{
		{"docker.io/library/rabbitmq:4.2.3", "rabbitmq"},
		{"quay.io/prometheus/prometheus:v3.2.1", "prometheus"},
		{"ghcr.io/kiwigrid/k8s-sidecar:1.28.0", "k8s-sidecar"},
		{"nginx:1.25", "nginx"},
		{"quay.io/some-org/tool@sha256:abc123", "tool"},
	}

	for _, tc := range tests {
		t.Run(tc.ref, func(t *testing.T) {
			got := nameBasename(tc.ref)
			if got != tc.want {
				t.Errorf("nameBasename(%q) = %q, want %q", tc.ref, got, tc.want)
			}
		})
	}
}

func TestIsExcluded(t *testing.T) {
	exclude := map[string]struct{}{"prometheus": {}, "rabbitmq": {}}

	tests := []struct {
		name string
		img  DiscoveredImage
		want bool
	}{
		{
			name: "basename fallback match for prometheus path",
			img:  DiscoveredImage{Name: "prometheus/prometheus", Source: "quay.io/prometheus/prometheus:v3"},
			want: true,
		},
		{
			name: "basename fallback match",
			img:  DiscoveredImage{Name: "library/rabbitmq", Source: "docker.io/library/rabbitmq:4.2.3"},
			want: true,
		},
		{
			name: "no match",
			img:  DiscoveredImage{Name: "kiwigrid/k8s-sidecar", Source: "ghcr.io/kiwigrid/k8s-sidecar:1.28.0"},
			want: false,
		},
		{
			name: "nil exclude set",
			img:  DiscoveredImage{Name: "prometheus", Source: "quay.io/prometheus/prometheus:v3"},
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			exc := exclude
			if tc.name == "nil exclude set" {
				exc = nil
			}
			if got := isExcluded(&tc.img, exc); got != tc.want {
				t.Errorf("isExcluded(%+v) = %v, want %v", tc.img, got, tc.want)
			}
		})
	}

	// Slash-to-hyphen flatten match: chart-discovered name "kyverno/background-controller"
	// must match a Wolfi rebuild stub named "kyverno-background-controller". Without this
	// normalization neither exact-match nor basename ("background-controller") would match.
	t.Run("slash-to-hyphen flatten match", func(t *testing.T) {
		exc := map[string]struct{}{"kyverno-background-controller": {}}
		img := DiscoveredImage{
			Name:   "kyverno/background-controller",
			Source: "reg.kyverno.io/kyverno/background-controller:v1.17.2",
		}
		if !isExcluded(&img, exc) {
			t.Errorf("isExcluded(%+v) = false, want true (flatten should match)", img)
		}
	})

	// Slash-to-hyphen flatten must NOT match if neither variant is present.
	t.Run("flatten miss does not match", func(t *testing.T) {
		exc := map[string]struct{}{"unrelated": {}}
		img := DiscoveredImage{Name: "kyverno/background-controller"}
		if isExcluded(&img, exc) {
			t.Errorf("isExcluded(%+v) = true, want false", img)
		}
	})
}

// Regression test for the review feedback that unpatchable entries from
// verity.yaml were being merged into excludeNames and emitting the
// "excluded via --exclude-names" log line, mis-attributing the source.
// Discover() now takes unpatchable as a separate parameter and applies it
// to BOTH standalone and chart-discovered images with a distinct skip path.
func TestDiscover_UnpatchableStandalone(t *testing.T) {
	cfg := &config.CopaConfig{
		Target: config.TargetSpec{Registry: "ghcr.io/verity-org"},
		Images: []config.ImageSpec{
			{
				Name:  "cert-manager/cert-manager-openshift-routes",
				Image: "ghcr.io/cert-manager/cert-manager-openshift-routes",
				Tags:  config.TagStrategy{Strategy: "list", List: []string{"v0.9.0"}},
			},
			{
				Name:  testNginxName,
				Image: "mirror.gcr.io/library/nginx",
				Tags:  config.TagStrategy{Strategy: "list", List: []string{"1.25.3"}},
			},
		},
	}

	unpatchable := map[string]struct{}{
		"cert-manager/cert-manager-openshift-routes": {},
	}

	got, err := Discover(cfg, "", nil, nil, unpatchable)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}

	if len(got) != 1 || got[0].Name != testNginxName {
		t.Errorf("Discover() returned %v, want only [{nginx}]; unpatchable standalone leaked through", got)
	}
}

// isUnpatchable must NOT do basename or slash-flatten fallback — those are
// reserved for the Wolfi-rebuild excludeNames path, which has different
// semantics. A standalone "library/nginx" listed as "library/nginx" must not
// be matched by an unpatchable entry "nginx".
func TestIsUnpatchable_ExactMatchOnly(t *testing.T) {
	tests := []struct {
		name        string
		imgName     string
		unpatchable map[string]struct{}
		want        bool
	}{
		{
			name:        "exact match",
			imgName:     "kyverno/cleanup-controller",
			unpatchable: map[string]struct{}{"kyverno/cleanup-controller": {}},
			want:        true,
		},
		{
			name:        "basename does NOT match (unlike isExcluded)",
			imgName:     "library/nginx",
			unpatchable: map[string]struct{}{"nginx": {}},
			want:        false,
		},
		{
			name:        "slash-flatten does NOT match (unlike isExcluded)",
			imgName:     "kyverno/cleanup-controller",
			unpatchable: map[string]struct{}{"kyverno-cleanup-controller": {}},
			want:        false,
		},
		{
			name:        "nil set",
			imgName:     "kyverno/cleanup-controller",
			unpatchable: nil,
			want:        false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			img := DiscoveredImage{Name: tc.imgName}
			if got := isUnpatchable(&img, tc.unpatchable); got != tc.want {
				t.Errorf("isUnpatchable(%q, %v) = %v, want %v", tc.imgName, tc.unpatchable, got, tc.want)
			}
		})
	}
}
