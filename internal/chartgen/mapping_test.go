package chartgen

import (
	"os"
	"path/filepath"
	"testing"
)

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
			name: "org differs from name",
			ref:  "ghcr.io/kiwigrid/k8s-sidecar:1.28.0",
			want: "kiwigrid/k8s-sidecar",
		},
		{
			name: "simple image",
			ref:  "nginx:1.25",
			want: "nginx",
		},
		{
			name: "different registry same basename",
			ref:  "quay.io/some-org/nginx:1.29",
			want: "some-org/nginx",
		},
		{
			name: "repeated component",
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

func TestSplitRef(t *testing.T) {
	tests := []struct {
		name     string
		ref      string
		wantName string
		wantTag  string
	}{
		{
			name:     "name and tag",
			ref:      "quay.io/prom/prometheus:v3.2.1",
			wantName: "quay.io/prom/prometheus",
			wantTag:  "v3.2.1",
		},
		{
			name:     "name only",
			ref:      "nginx",
			wantName: "nginx",
			wantTag:  "",
		},
		{
			name:     "digest stripped",
			ref:      "quay.io/foo/bar:v1.2@sha256:abc123",
			wantName: "quay.io/foo/bar",
			wantTag:  "v1.2",
		},
		{
			name:     "digest only no tag",
			ref:      "quay.io/foo/bar@sha256:abc123",
			wantName: "quay.io/foo/bar",
			wantTag:  "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotName, gotTag := splitRef(tc.ref)
			if gotName != tc.wantName || gotTag != tc.wantTag {
				t.Errorf("splitRef(%q) = (%q, %q), want (%q, %q)", tc.ref, gotName, gotTag, tc.wantName, tc.wantTag)
			}
		})
	}
}

func TestBuildImageMappings(t *testing.T) {
	tmpDir := t.TempDir()
	fakeCrane := filepath.Join(tmpDir, "crane")
	script := `#!/bin/sh
if [ "$1" != "ls" ]; then
  exit 1
fi

case "$2" in
  "ghcr.io/verity-org/prometheus/prometheus")
    printf "v3.2.1\nlatest\n"
    ;;
  "ghcr.io/verity-org/kiwigrid/k8s-sidecar")
    printf "1.28.0\n"
    ;;
  "ghcr.io/verity-org/library/no-patch")
    printf "latest\n"
    ;;
  *)
    printf "\n"
    ;;
esac
`
	if err := os.WriteFile(fakeCrane, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile(fake crane) error = %v", err)
	}

	origPath := os.Getenv("PATH")
	t.Setenv("PATH", tmpDir+string(os.PathListSeparator)+origPath)

	imageRefs := []string{
		"quay.io/prometheus/prometheus:v3.2.1",
		"ghcr.io/kiwigrid/k8s-sidecar:1.28.0",
		"docker.io/library/no-patch:1.0.0",
	}

	excludeNames := map[string]struct{}{
		"k8s-sidecar": {},
	}

	got, err := BuildImageMappings(imageRefs, "ghcr.io/verity-org", excludeNames)
	if err != nil {
		t.Fatalf("BuildImageMappings() error = %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("BuildImageMappings() returned %d mappings, want 1", len(got))
	}

	want := ImageMapping{
		OriginalRepo: "quay.io/prometheus/prometheus",
		OriginalTag:  "v3.2.1",
		PatchedRepo:  "ghcr.io/verity-org/prometheus/prometheus",
		PatchedTag:   "v3.2.1",
	}

	if got[0] != want {
		t.Errorf("BuildImageMappings()[0] = %+v, want %+v", got[0], want)
	}
}

func TestIsExcluded(t *testing.T) {
	exclude := map[string]struct{}{"rabbitmq": {}, "k8s-sidecar": {}}

	tests := []struct {
		name     string
		imgName  string
		imageRef string
		want     bool
	}{
		{
			name:     "basename fallback match for path-preserving name",
			imgName:  "kiwigrid/k8s-sidecar",
			imageRef: "ghcr.io/kiwigrid/k8s-sidecar:1.28.0",
			want:     true,
		},
		{
			name:     "basename fallback match",
			imgName:  "library/rabbitmq",
			imageRef: "docker.io/library/rabbitmq:4.2.3",
			want:     true,
		},
		{
			name:     "no match",
			imgName:  "prometheus/prometheus",
			imageRef: "quay.io/prometheus/prometheus:v3",
			want:     false,
		},
		{
			name:     "nil exclude set",
			imgName:  "rabbitmq",
			imageRef: "docker.io/library/rabbitmq:4.2.3",
			want:     false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			exc := exclude
			if tc.name == "nil exclude set" {
				exc = nil
			}
			got := isExcluded(tc.imgName, exc)
			if got != tc.want {
				t.Errorf("isExcluded(%q, %q) = %v, want %v", tc.imgName, tc.imageRef, got, tc.want)
			}
		})
	}
}
