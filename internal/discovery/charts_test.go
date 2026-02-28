package discovery

import (
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

func TestExtractImagesFromManifests(t *testing.T) {
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

	// Should have 3 unique images (deduplication of the repeated prometheus ref)
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
}
