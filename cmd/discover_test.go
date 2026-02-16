package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/verity-org/verity/internal"
)

func TestDiscoverCommand_ParseImages(t *testing.T) {
	// Create a temp directory for test outputs
	tmpDir := t.TempDir()

	// Create a minimal values.yaml
	valuesPath := filepath.Join(tmpDir, "values.yaml")
	valuesContent := `nginx:
  image:
    registry: docker.io
    repository: library/nginx
    tag: "1.25"
`
	if err := os.WriteFile(valuesPath, []byte(valuesContent), 0o644); err != nil {
		t.Fatalf("failed to create test values.yaml: %v", err)
	}

	// Parse the images
	images, err := internal.ParseImagesFile(valuesPath)
	if err != nil {
		t.Fatalf("failed to parse images: %v", err)
	}

	if len(images) != 1 {
		t.Errorf("expected 1 image, got %d", len(images))
	}

	if images[0].Reference() != "docker.io/library/nginx:1.25" {
		t.Errorf("expected reference 'docker.io/library/nginx:1.25', got %q", images[0].Reference())
	}
}

func TestDiscoverCommand_MultipleImages(t *testing.T) {
	tmpDir := t.TempDir()
	valuesPath := filepath.Join(tmpDir, "values.yaml")
	valuesContent := `nginx:
  image:
    registry: docker.io
    repository: library/nginx
    tag: "1.25"
prometheus:
  image:
    registry: quay.io
    repository: prometheus/prometheus
    tag: "v2.45.0"
`
	if err := os.WriteFile(valuesPath, []byte(valuesContent), 0o644); err != nil {
		t.Fatalf("failed to create test values.yaml: %v", err)
	}

	images, err := internal.ParseImagesFile(valuesPath)
	if err != nil {
		t.Fatalf("failed to parse images: %v", err)
	}

	if len(images) != 2 {
		t.Errorf("expected 2 images, got %d", len(images))
	}
}
