package internal

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindImages(t *testing.T) {
	values := map[string]interface{}{
		"server": map[string]interface{}{
			"image": map[string]interface{}{
				"repository": "quay.io/prometheus/prometheus",
				"tag":        "v2.48.0",
			},
			"replicas": 1,
		},
		"alertmanager": map[string]interface{}{
			"image": map[string]interface{}{
				"registry":   "docker.io",
				"repository": "prom/alertmanager",
				"tag":        "v0.26.0",
			},
		},
		"nodeExporter": map[string]interface{}{
			"image": map[string]interface{}{
				"repository": "quay.io/prometheus/node-exporter",
				"tag":        "v1.7.0",
			},
		},
		"configmapReload": map[string]interface{}{
			"image": "jimmidyson/configmap-reload:v0.12.0",
		},
		"notAnImage": map[string]interface{}{
			"repository": "https://example.com/charts",
		},
	}

	images := findImages(values, "", "", nil)

	if len(images) < 3 {
		t.Fatalf("expected at least 3 images, got %d", len(images))
	}

	refs := map[string]bool{}
	for _, img := range images {
		refs[img.Reference()] = true
		t.Logf("found: %s (path: %s)", img.Reference(), img.Path)
	}

	expected := []string{
		"quay.io/prometheus/prometheus:v2.48.0",
		"docker.io/prom/alertmanager:v0.26.0",
		"quay.io/prometheus/node-exporter:v1.7.0",
	}
	for _, e := range expected {
		if !refs[e] {
			t.Errorf("expected image %q not found", e)
		}
	}

	// URL should NOT be detected as an image
	for _, img := range images {
		if img.Repository == "https://example.com/charts" {
			t.Error("URL was incorrectly detected as an image")
		}
	}
}

func TestParseRef(t *testing.T) {
	tests := []struct {
		input string
		want  Image
	}{
		{
			input: "nginx:1.25",
			want:  Image{Repository: "nginx", Tag: "1.25"},
		},
		{
			input: "quay.io/prometheus/prometheus:v2.48.0",
			want:  Image{Registry: "quay.io", Repository: "prometheus/prometheus", Tag: "v2.48.0"},
		},
		{
			input: "docker.io/library/nginx:latest",
			want:  Image{Registry: "docker.io", Repository: "library/nginx", Tag: "latest"},
		},
	}

	for _, tt := range tests {
		got := parseRef(tt.input)
		if got.Registry != tt.want.Registry || got.Repository != tt.want.Repository || got.Tag != tt.want.Tag {
			t.Errorf("parseRef(%q) = %+v, want %+v", tt.input, got, tt.want)
		}
	}
}

func TestLooksLikeImage(t *testing.T) {
	yes := []string{"prom/prometheus", "nginx", "quay.io/something"}
	no := []string{"", "true", "https://example.com", "some thing with spaces"}

	for _, s := range yes {
		if !looksLikeImage(s) {
			t.Errorf("looksLikeImage(%q) should be true", s)
		}
	}
	for _, s := range no {
		if looksLikeImage(s) {
			t.Errorf("looksLikeImage(%q) should be false", s)
		}
	}
}

func TestFindImagesWithEmptyTag(t *testing.T) {
	// Test with appVersion that has "v" prefix — used as-is, no registry check needed
	values := map[string]interface{}{
		"server": map[string]interface{}{
			"image": map[string]interface{}{
				"repository": "quay.io/prometheus/prometheus",
				"tag":        "",
			},
		},
	}
	images := findImages(values, "", "v2.48.0", nil)
	refs := map[string]bool{}
	for _, img := range images {
		refs[img.Reference()] = true
	}
	if !refs["quay.io/prometheus/prometheus:v2.48.0"] {
		t.Errorf("expected image with tag from appVersion (v prefix): got %v", refs)
	}

	// Test resolveTag selecting "v" prefix when only v-prefixed tag exists
	orig := tagChecker
	defer func() { tagChecker = orig }()
	tagChecker = func(_ context.Context, ref string) bool {
		// Simulate: only v-prefixed tag exists
		return strings.HasSuffix(ref, ":v2.10.1")
	}

	values = map[string]interface{}{
		"kube-state-metrics": map[string]interface{}{
			"image": map[string]interface{}{
				"registry":   "registry.k8s.io",
				"repository": "kube-state-metrics/kube-state-metrics",
				"tag":        "",
			},
		},
	}
	images = findImages(values, "", "2.10.1", nil)
	refs = map[string]bool{}
	for _, img := range images {
		refs[img.Reference()] = true
	}
	if !refs["registry.k8s.io/kube-state-metrics/kube-state-metrics:v2.10.1"] {
		t.Errorf("expected v-prefixed tag when only v variant exists: got %v", refs)
	}

	// Test resolveTag using appVersion as-is when it exists without "v"
	tagChecker = func(_ context.Context, ref string) bool {
		// Simulate: only non-v tag exists
		return strings.HasSuffix(ref, ":0.50.0-distroless-libc")
	}

	values = map[string]interface{}{
		"vector": map[string]interface{}{
			"image": map[string]interface{}{
				"repository": "timberio/vector",
				"tag":        "",
			},
		},
	}
	images = findImages(values, "", "0.50.0-distroless-libc", nil)
	refs = map[string]bool{}
	for _, img := range images {
		refs[img.Reference()] = true
	}
	if !refs["timberio/vector:0.50.0-distroless-libc"] {
		t.Errorf("expected appVersion as-is when it exists: got %v", refs)
	}

	// Test fallback to as-is when registry is unreachable
	tagChecker = func(_ context.Context, _ string) bool { return false }

	images = findImages(values, "", "9.9.9", nil)
	refs = map[string]bool{}
	for _, img := range images {
		refs[img.Reference()] = true
	}
	if !refs["timberio/vector:9.9.9"] {
		t.Errorf("expected appVersion as-is when registry unreachable: got %v", refs)
	}
}

func TestParseImagesFile(t *testing.T) {
	content := `
nginx:
  image:
    registry: docker.io
    repository: library/nginx
    tag: "1.25.0"
redis:
  image:
    registry: docker.io
    repository: library/redis
    tag: "7.2.0"
postgres:
  image:
    registry: docker.io
    repository: library/postgres
    tag: "16.1"
`
	dir := t.TempDir()
	path := filepath.Join(dir, "values.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	images, err := ParseImagesFile(path)
	if err != nil {
		t.Fatalf("ParseImagesFile() error: %v", err)
	}

	if len(images) != 3 {
		t.Fatalf("expected 3 images, got %d", len(images))
	}

	refs := map[string]bool{}
	for _, img := range images {
		refs[img.Reference()] = true
	}

	expected := []string{
		"docker.io/library/nginx:1.25.0",
		"docker.io/library/redis:7.2.0",
		"docker.io/library/postgres:16.1",
	}
	for _, e := range expected {
		if !refs[e] {
			t.Errorf("expected image %q not found in %v", e, refs)
		}
	}
}

func TestParseImagesFileEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "values.yaml")
	if err := os.WriteFile(path, []byte("# empty\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	images, err := ParseImagesFile(path)
	if err != nil {
		t.Fatalf("ParseImagesFile() error: %v", err)
	}
	if len(images) != 0 {
		t.Errorf("expected 0 images from empty file, got %d", len(images))
	}
}

func TestParseImagesFileMissing(t *testing.T) {
	_, err := ParseImagesFile("/nonexistent/values.yaml")
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
}

func TestParseOverrides(t *testing.T) {
	content := `
overrides:
  timberio/vector:
    from: "distroless-libc"
    to: "debian"
  some/image:
    from: "scratch"
    to: "alpine"

nginx:
  image:
    registry: docker.io
    repository: library/nginx
    tag: "1.25.0"
`
	dir := t.TempDir()
	path := filepath.Join(dir, "values.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	overrides, err := ParseOverrides(path)
	if err != nil {
		t.Fatalf("ParseOverrides() error: %v", err)
	}

	if len(overrides) != 2 {
		t.Fatalf("expected 2 overrides, got %d", len(overrides))
	}

	found := map[string]ImageOverride{}
	for _, o := range overrides {
		found[o.Repository] = o
	}

	vec, ok := found["timberio/vector"]
	if !ok {
		t.Fatal("expected override for timberio/vector")
	}
	if vec.From != "distroless-libc" || vec.To != "debian" {
		t.Errorf("unexpected vector override: %+v", vec)
	}

	// Verify images are still parsed correctly alongside overrides
	images, err := ParseImagesFile(path)
	if err != nil {
		t.Fatalf("ParseImagesFile() error: %v", err)
	}
	if len(images) != 1 {
		t.Errorf("expected 1 image (nginx), got %d", len(images))
	}
}

func TestParseOverridesNoSection(t *testing.T) {
	content := `
nginx:
  image:
    registry: docker.io
    repository: library/nginx
    tag: "1.25.0"
`
	dir := t.TempDir()
	path := filepath.Join(dir, "values.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	overrides, err := ParseOverrides(path)
	if err != nil {
		t.Fatalf("ParseOverrides() error: %v", err)
	}
	if len(overrides) != 0 {
		t.Errorf("expected 0 overrides when no section, got %d", len(overrides))
	}
}

func TestApplyOverrides(t *testing.T) {
	images := []Image{
		{Repository: "timberio/vector", Tag: "0.46.1-distroless-libc", Path: "vector.image"},
		{Registry: "docker.io", Repository: "library/nginx", Tag: "1.25.0", Path: "nginx.image"},
		{Repository: "victoriametrics/victoria-logs", Tag: "v1.0.0-victorialogs", Path: "server.image"},
	}

	overrides := []ImageOverride{
		{Repository: "timberio/vector", From: "distroless-libc", To: "debian"},
	}

	result := ApplyOverrides(images, overrides)

	if result[0].Tag != "0.46.1-debian" {
		t.Errorf("expected vector tag 0.46.1-debian, got %s", result[0].Tag)
	}
	if result[1].Tag != "1.25.0" {
		t.Errorf("nginx tag should be unchanged, got %s", result[1].Tag)
	}
	if result[2].Tag != "v1.0.0-victorialogs" {
		t.Errorf("victoria-logs tag should be unchanged, got %s", result[2].Tag)
	}
}

func TestApplyOverridesWithRegistry(t *testing.T) {
	images := []Image{
		{Registry: "docker.io", Repository: "timberio/vector", Tag: "0.46.1-distroless-libc", Path: "vector.image"},
	}

	overrides := []ImageOverride{
		{Repository: "docker.io/timberio/vector", From: "distroless-libc", To: "debian"},
	}

	result := ApplyOverrides(images, overrides)

	if result[0].Tag != "0.46.1-debian" {
		t.Errorf("expected vector tag 0.46.1-debian, got %s", result[0].Tag)
	}
}

func TestApplyOverridesEmpty(t *testing.T) {
	images := []Image{
		{Repository: "nginx", Tag: "1.25.0"},
	}

	result := ApplyOverrides(images, nil)

	if result[0].Tag != "1.25.0" {
		t.Errorf("expected unchanged tag, got %s", result[0].Tag)
	}
}
