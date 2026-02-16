package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/verity-org/verity/internal"
)

func TestListCommand_ParseImages(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		wantCount   int
		wantFirst   string
		wantErr     bool
	}{
		{
			name: "single image",
			content: `nginx:
  image:
    registry: docker.io
    repository: library/nginx
    tag: "1.25"
`,
			wantCount: 1,
			wantFirst: "docker.io/library/nginx:1.25",
			wantErr:   false,
		},
		{
			name: "multiple images",
			content: `nginx:
  image:
    registry: docker.io
    repository: library/nginx
    tag: "1.25"
prometheus:
  image:
    registry: quay.io
    repository: prometheus/prometheus
    tag: "v2.45.0"
`,
			wantCount: 2,
			wantFirst: "docker.io/library/nginx:1.25",
			wantErr:   false,
		},
		{
			name: "image without registry",
			content: `nginx:
  image:
    repository: library/nginx
    tag: "1.25"
`,
			wantCount: 1,
			wantFirst: "library/nginx:1.25",
			wantErr:   false,
		},
		{
			name: "empty file",
			content: ``,
			wantCount: 0,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			valuesPath := filepath.Join(tmpDir, "values.yaml")

			if err := os.WriteFile(valuesPath, []byte(tt.content), 0o644); err != nil {
				t.Fatalf("failed to create test file: %v", err)
			}

			images, err := internal.ParseImagesFile(valuesPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseImagesFile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if len(images) != tt.wantCount {
				t.Errorf("got %d images, want %d", len(images), tt.wantCount)
			}

			if tt.wantCount > 0 && images[0].Reference() != tt.wantFirst {
				t.Errorf("first image reference = %q, want %q", images[0].Reference(), tt.wantFirst)
			}
		})
	}
}
