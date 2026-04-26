package discovery

import (
	"errors"
	"testing"

	"github.com/verity-org/verity/internal/config"
)

func TestApplyOverride(t *testing.T) {
	overrides := map[string]config.Override{
		"timberio/vector": {From: "distroless-libc", To: "debian"},
	}

	tests := []struct {
		name  string
		image string
		want  string
	}{
		{
			name:  "applies override to matching image",
			image: "docker.io/timberio/vector:0.43.0-distroless-libc",
			want:  "docker.io/timberio/vector:0.43.0-debian",
		},
		{
			name:  "no change when image doesn't match key",
			image: "docker.io/grafana/grafana:10.0.0",
			want:  "docker.io/grafana/grafana:10.0.0",
		},
		{
			name:  "no change when from suffix not present",
			image: "docker.io/timberio/vector:0.43.0-alpine",
			want:  "docker.io/timberio/vector:0.43.0-alpine",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := applyOverride(tc.image, overrides)
			if got != tc.want {
				t.Errorf("applyOverride(%q) = %q, want %q", tc.image, got, tc.want)
			}
		})
	}
}

func TestHelmTemplateArgs(t *testing.T) {
	tests := []struct {
		name  string
		chart config.ChartSpec
		want  []string
	}{
		{
			name: "OCI repository",
			chart: config.ChartSpec{
				Name:       "prometheus",
				Version:    "28.9.1",
				Repository: "oci://ghcr.io/prometheus-community/charts",
			},
			want: []string{
				"template", "prometheus",
				"oci://ghcr.io/prometheus-community/charts/prometheus",
				"--version", "28.9.1",
			},
		},
		{
			name: "HTTP repository",
			chart: config.ChartSpec{
				Name:       "postgres-operator",
				Version:    "1.15.1",
				Repository: "https://opensource.zalando.com/postgres-operator/charts/postgres-operator",
			},
			want: []string{
				"template", "postgres-operator", "postgres-operator",
				"--repo", "https://opensource.zalando.com/postgres-operator/charts/postgres-operator",
				"--version", "1.15.1",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := helmTemplateArgs(tc.chart)
			if len(got) != len(tc.want) {
				t.Fatalf("helmTemplateArgs() = %v, want %v", got, tc.want)
			}
			for i, g := range got {
				if g != tc.want[i] {
					t.Errorf("helmTemplateArgs()[%d] = %q, want %q", i, g, tc.want[i])
				}
			}
		})
	}
}

func TestValidateChartSpec(t *testing.T) {
	tests := []struct {
		name    string
		chart   config.ChartSpec
		wantErr error
	}{
		{
			name:    "valid OCI chart",
			chart:   config.ChartSpec{Name: "prometheus", Version: "28.9.1", Repository: "oci://ghcr.io/charts"},
			wantErr: nil,
		},
		{
			name:    "valid HTTPS chart",
			chart:   config.ChartSpec{Name: "grafana", Version: "8.0.0", Repository: "https://grafana.github.io/helm-charts"},
			wantErr: nil,
		},
		{
			name:    "valid HTTP chart",
			chart:   config.ChartSpec{Name: "myapp", Version: "1.0.0", Repository: "http://charts.example.com"},
			wantErr: nil,
		},
		{
			name:    "injection via name with dash-dash",
			chart:   config.ChartSpec{Name: "--set-file", Version: "1.0", Repository: "oci://ghcr.io/charts"},
			wantErr: ErrInvalidChartName,
		},
		{
			name:    "injection via name with single dash",
			chart:   config.ChartSpec{Name: "-n", Version: "1.0", Repository: "oci://ghcr.io/charts"},
			wantErr: ErrInvalidChartName,
		},
		{
			name:    "injection via version with dash-dash",
			chart:   config.ChartSpec{Name: "app", Version: "--post-renderer", Repository: "oci://ghcr.io/charts"},
			wantErr: ErrInvalidChartVersion,
		},
		{
			name:    "injection via version with single dash",
			chart:   config.ChartSpec{Name: "app", Version: "-x", Repository: "oci://ghcr.io/charts"},
			wantErr: ErrInvalidChartVersion,
		},
		{
			name:    "invalid repo scheme file://",
			chart:   config.ChartSpec{Name: "app", Version: "1.0", Repository: "file:///etc/passwd"},
			wantErr: ErrInvalidChartRepo,
		},
		{
			name:    "invalid repo empty",
			chart:   config.ChartSpec{Name: "app", Version: "1.0", Repository: ""},
			wantErr: ErrInvalidChartRepo,
		},
		{
			name:    "invalid repo bare path",
			chart:   config.ChartSpec{Name: "app", Version: "1.0", Repository: "/tmp/evil"},
			wantErr: ErrInvalidChartRepo,
		},
		{
			name:    "invalid repo ftp",
			chart:   config.ChartSpec{Name: "app", Version: "1.0", Repository: "ftp://evil.com"},
			wantErr: ErrInvalidChartRepo,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateChartSpec(tc.chart)
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("ValidateChartSpec() unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("ValidateChartSpec() expected error wrapping %v, got nil", tc.wantErr)
			}
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("ValidateChartSpec() error = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestSplitRef(t *testing.T) {
	tests := []struct {
		ref      string
		wantName string
		wantTag  string
	}{
		{"docker.io/foo/bar:1.0-alpine", "docker.io/foo/bar", "1.0-alpine"},
		{"nginx:latest", "nginx", "latest"},
		{"ghcr.io/verity-org/nginx", "ghcr.io/verity-org/nginx", ""},
		{"localhost:5000/myimage:v1", "localhost:5000/myimage", "v1"},
		{"registry.example.com:8080/repo:tag", "registry.example.com:8080/repo", "tag"},
		{"simple", "simple", ""},
	}
	for _, tc := range tests {
		t.Run(tc.ref, func(t *testing.T) {
			name, tag := splitRef(tc.ref)
			if name != tc.wantName || tag != tc.wantTag {
				t.Errorf("splitRef(%q) = (%q, %q), want (%q, %q)", tc.ref, name, tag, tc.wantName, tc.wantTag)
			}
		})
	}
}

func TestExtractImagesFromManifests(t *testing.T) {
	t.Run("deduplicates image fields across manifests", func(t *testing.T) {
		yaml := []byte(`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: prometheus
spec:
  template:
    spec:
      containers:
      - name: prometheus
        image: quay.io/prometheus/prometheus:v3.2.1
      - name: configmap-reload
        image: ghcr.io/jimmidyson/configmap-reload:v0.14.0
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: alertmanager
spec:
  template:
    spec:
      containers:
      - name: alertmanager
        image: quay.io/prometheus/alertmanager:v0.28.1
      - name: duplicate
        image: quay.io/prometheus/prometheus:v3.2.1
`)

		got, err := extractImagesFromManifests(yaml)
		if err != nil {
			t.Fatalf("extractImagesFromManifests() error = %v", err)
		}

		if len(got) != 3 {
			t.Errorf("extractImagesFromManifests() returned %d images, want 3: %v", len(got), got)
		}

		wantImages := map[string]bool{
			"quay.io/prometheus/prometheus:v3.2.1":        true,
			"ghcr.io/jimmidyson/configmap-reload:v0.14.0": true,
			"quay.io/prometheus/alertmanager:v0.28.1":     true,
		}
		for _, img := range got {
			if !wantImages[img] {
				t.Errorf("unexpected image: %q", img)
			}
		}
	})

	t.Run("keeps existing image key discovery", func(t *testing.T) {
		yaml := []byte(`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: tiny
spec:
  template:
    spec:
      containers:
      - name: tiny
        image: ghcr.io/verity-org/tiny:v1.2.3
`)

		got, err := extractImagesFromManifests(yaml)
		if err != nil {
			t.Fatalf("extractImagesFromManifests() error = %v", err)
		}

		if len(got) != 1 {
			t.Fatalf("extractImagesFromManifests() returned %d images, want 1: %v", len(got), got)
		}
		if got[0] != "ghcr.io/verity-org/tiny:v1.2.3" {
			t.Fatalf("extractImagesFromManifests() = %v, want ghcr.io/verity-org/tiny:v1.2.3", got)
		}
	})
}
