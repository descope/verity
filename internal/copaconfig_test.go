package internal

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseImageRef(t *testing.T) {
	tests := []struct {
		name           string
		ref            string
		wantRegistry   string
		wantRepository string
		wantTag        string
	}{
		{
			name:           "full reference with tag",
			ref:            "ghcr.io/verity-org/library/nginx:1.25.3-patched",
			wantRegistry:   "ghcr.io",
			wantRepository: "verity-org/library/nginx",
			wantTag:        "1.25.3-patched",
		},
		{
			name:           "docker hub with library",
			ref:            "docker.io/library/nginx:1.25.3",
			wantRegistry:   "docker.io",
			wantRepository: "library/nginx",
			wantTag:        "1.25.3",
		},
		{
			name:           "no registry",
			ref:            "library/nginx:1.25.3",
			wantRegistry:   "",
			wantRepository: "library/nginx",
			wantTag:        "1.25.3",
		},
		{
			name:           "quay.io",
			ref:            "quay.io/prometheus/prometheus:v3.9.1",
			wantRegistry:   "quay.io",
			wantRepository: "prometheus/prometheus",
			wantTag:        "v3.9.1",
		},
		{
			name:           "no tag",
			ref:            "ghcr.io/verity-org/nginx",
			wantRegistry:   "ghcr.io",
			wantRepository: "verity-org/nginx",
			wantTag:        "",
		},
		{
			name:           "localhost registry",
			ref:            "localhost:5000/myimage:latest",
			wantRegistry:   "localhost:5000",
			wantRepository: "myimage",
			wantTag:        "latest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRegistry, gotRepository, gotTag := ParseImageRef(tt.ref)
			if gotRegistry != tt.wantRegistry {
				t.Errorf("ParseImageRef() registry = %v, want %v", gotRegistry, tt.wantRegistry)
			}
			if gotRepository != tt.wantRepository {
				t.Errorf("ParseImageRef() repository = %v, want %v", gotRepository, tt.wantRepository)
			}
			if gotTag != tt.wantTag {
				t.Errorf("ParseImageRef() tag = %v, want %v", gotTag, tt.wantTag)
			}
		})
	}
}

func TestNormalizeImageRef(t *testing.T) {
	tests := []struct {
		name string
		ref  string
		want string
	}{
		{
			name: "nginx short form",
			ref:  "nginx:1.25.3",
			want: "docker.io/library/nginx:1.25.3",
		},
		{
			name: "library prefix already present",
			ref:  "library/nginx:1.25.3",
			want: "docker.io/library/nginx:1.25.3",
		},
		{
			name: "already normalized",
			ref:  "docker.io/library/nginx:1.25.3",
			want: "docker.io/library/nginx:1.25.3",
		},
		{
			name: "quay.io image",
			ref:  "quay.io/prometheus/prometheus:v3.9.1",
			want: "quay.io/prometheus/prometheus:v3.9.1",
		},
		{
			name: "ghcr.io image",
			ref:  "ghcr.io/verity-org/nginx:1.25.3",
			want: "ghcr.io/verity-org/nginx:1.25.3",
		},
		{
			name: "docker hub user image",
			ref:  "grafana/grafana:12.3.3",
			want: "docker.io/grafana/grafana:12.3.3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeImageRef(tt.ref); got != tt.want {
				t.Errorf("NormalizeImageRef() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseCopaOutput(t *testing.T) {
	tmpDir := t.TempDir()
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
    }
  ]
}`

	if err := os.WriteFile(copaOutputPath, []byte(copaJSON), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	output, err := ParseCopaOutput(copaOutputPath)
	if err != nil {
		t.Fatalf("ParseCopaOutput() error = %v", err)
	}

	if len(output.Results) != 2 {
		t.Errorf("ParseCopaOutput() got %d results, want 2", len(output.Results))
	}

	if output.Results[0].Name != "nginx" {
		t.Errorf("ParseCopaOutput() result[0].Name = %v, want nginx", output.Results[0].Name)
	}
	if output.Results[0].Status != "Patched" {
		t.Errorf("ParseCopaOutput() result[0].Status = %v, want Patched", output.Results[0].Status)
	}

	if output.Results[1].Status != "Skipped" {
		t.Errorf("ParseCopaOutput() result[1].Status = %v, want Skipped", output.Results[1].Status)
	}
}
