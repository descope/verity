//go:build integration

package integration

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAllowlist(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "alist.txt")
	body := "" +
		"# header comment\n" +
		"\n" +
		"docker.io/library/something\n" +
		"  quay.io/special/image  \n" +
		"# trailing comment\n" +
		"registry.k8s.io/foo\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := loadAllowlist(path)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"docker.io/library/something",
		"quay.io/special/image",
		"registry.k8s.io/foo",
	}
	if len(got) != len(want) {
		t.Fatalf("len got=%d want=%d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("idx=%d got=%q want=%q", i, got[i], want[i])
		}
	}
}

func TestLoadAllowlistMissing(t *testing.T) {
	got, err := loadAllowlist(filepath.Join(t.TempDir(), "does-not-exist.txt"))
	if err != nil {
		t.Fatalf("missing file should return nil, nil: got %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil slice, got %v", got)
	}
}

func TestIsAccepted(t *testing.T) {
	allow := []string{
		"quay.io/special/escape-hatch",
		"registry.k8s.io/known-cant-rewrite",
	}
	cases := []struct {
		name  string
		image string
		want  bool
	}{
		{"verity registry", "ghcr.io/verity-org/prometheus/prometheus:v3.9.1", true},
		{"verity registry verbose", "ghcr.io/verity-org/library/nginx:1.29.5-patched", true},
		{"allowlist exact", "quay.io/special/escape-hatch:latest", true},
		{"allowlist prefix", "registry.k8s.io/known-cant-rewrite/sub:v1", true},
		{"upstream rejected", "quay.io/prometheus/prometheus:v3.9.1", false},
		{"docker hub rejected", "docker.io/library/postgres:17", false},
		{"empty rejected", "", false},
		{"double-ghcr (chart-gen bug pattern) rejected", "ghcr.io/ghcr.io/verity-org/foo:tag", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := isAccepted(c.image, allow)
			if got != c.want {
				t.Fatalf("isAccepted(%q) = %v, want %v", c.image, got, c.want)
			}
		})
	}
}
