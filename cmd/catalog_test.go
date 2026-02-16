package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/verity-org/verity/internal"
)

func TestCatalogCommand_ParseImages(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test values.yaml
	valuesPath := filepath.Join(tmpDir, "values.yaml")
	valuesContent := `nginx:
  image:
    registry: docker.io
    repository: library/nginx
    tag: "1.25"
`
	if err := os.WriteFile(valuesPath, []byte(valuesContent), 0o644); err != nil {
		t.Fatalf("failed to create values.yaml: %v", err)
	}

	// Parse images
	images, err := internal.ParseImagesFile(valuesPath)
	if err != nil {
		t.Fatalf("failed to parse images: %v", err)
	}

	if len(images) != 1 {
		t.Errorf("expected 1 image, got %d", len(images))
	}
}

func TestCatalogCommand_ValidateFlags(t *testing.T) {
	tests := []struct {
		name       string
		images     string
		registry   string
		outputPath string
		wantErr    bool
	}{
		{
			name:       "all required flags",
			images:     "values.yaml",
			registry:   "ghcr.io/verity-org",
			outputPath: "catalog.json",
			wantErr:    false,
		},
		{
			name:       "missing images file",
			images:     "",
			registry:   "ghcr.io/verity-org",
			outputPath: "catalog.json",
			wantErr:    true,
		},
		{
			name:       "missing registry",
			images:     "values.yaml",
			registry:   "",
			outputPath: "catalog.json",
			wantErr:    true,
		},
		{
			name:       "missing output",
			images:     "values.yaml",
			registry:   "ghcr.io/verity-org",
			outputPath: "",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hasErr := tt.images == "" || tt.registry == "" || tt.outputPath == ""
			if hasErr != tt.wantErr {
				t.Errorf("validation error = %v, wantErr %v", hasErr, tt.wantErr)
			}
		})
	}
}
