package internal

import (
	"os"
	"path/filepath"
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

	images := findImages(values, "", "")

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
	values := map[string]interface{}{
		"server": map[string]interface{}{
			"image": map[string]interface{}{
				"repository": "quay.io/prometheus/prometheus",
				"tag":        "",
			},
		},
		"kube-state-metrics": map[string]interface{}{
			"image": map[string]interface{}{
				"registry":   "registry.k8s.io",
				"repository": "kube-state-metrics/kube-state-metrics",
				"tag":        "",
			},
		},
	}

	// Test with appVersion that has "v" prefix
	images := findImages(values, "", "v2.48.0")
	refs := map[string]bool{}
	for _, img := range images {
		refs[img.Reference()] = true
	}
	if !refs["quay.io/prometheus/prometheus:v2.48.0"] {
		t.Errorf("expected image with tag from appVersion (v prefix): got %v", refs)
	}

	// Test with appVersion without "v" prefix
	images = findImages(values, "", "2.10.1")
	refs = map[string]bool{}
	for _, img := range images {
		refs[img.Reference()] = true
	}
	if !refs["registry.k8s.io/kube-state-metrics/kube-state-metrics:v2.10.1"] {
		t.Errorf("expected image with tag from appVersion (no v prefix): got %v", refs)
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
