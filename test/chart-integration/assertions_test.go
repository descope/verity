//go:build integration

package integration

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/verity-org/verity/internal/config"
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
	// SCR-2026-04-30-001 (Option E): missing allowlist files are a
	// typed signal, not a silent empty allowlist. Callers must be
	// able to branch on errAllowlistMissing via errors.Is. The
	// returned slice is nil so the previous "(nil, nil)" callers
	// observe the same slice shape, just with a non-nil error.
	missingPath := filepath.Join(t.TempDir(), "does-not-exist.txt")
	got, err := loadAllowlist(missingPath)
	if err == nil {
		t.Fatalf("missing file must return errAllowlistMissing, got nil error (slice=%v)", got)
	}
	if !errors.Is(err, errAllowlistMissing) {
		t.Fatalf("missing file must return errAllowlistMissing (errors.Is), got %T: %v", err, err)
	}
	if got != nil {
		t.Fatalf("expected nil slice on missing file, got %v", got)
	}
	// The wrapped error must mention the requested path so
	// diagnostics in the test driver can name the missing file.
	if !strings.Contains(err.Error(), missingPath) {
		t.Fatalf("error message must contain requested path %q, got %q", missingPath, err.Error())
	}
}

func TestLoadAllowlistPresentRegression(t *testing.T) {
	// Regression: loadAllowlist still parses an existing file
	// correctly after the missing-file branch was promoted to a
	// typed error. Mirrors TestLoadAllowlist with a smaller body
	// to give the assertion a second, narrower data point.
	dir := t.TempDir()
	path := filepath.Join(dir, "alist.txt")
	body := "" +
		"# comment\n" +
		"\n" +
		"  registry.k8s.io/sig-storage/csi-provisioner  \n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := loadAllowlist(path)
	if err != nil {
		t.Fatalf("present file must not error: %v", err)
	}
	if errors.Is(err, errAllowlistMissing) {
		t.Fatalf("present file must NOT return errAllowlistMissing")
	}
	if len(got) != 1 || got[0] != "registry.k8s.io/sig-storage/csi-provisioner" {
		t.Fatalf("got %v, want exactly one trimmed entry", got)
	}
}

func TestClassifyMissingAllowlist(t *testing.T) {
	// SCR-2026-04-30-001 (Option E) policy decision table: the
	// driver hard-fails iff at least one observed image is non-
	// verity-prefixed. classifyMissingAllowlist returns the
	// offenders; an empty return = clean pass.
	cases := []struct {
		name      string
		images    []string
		offenders []string
	}{
		{
			name:      "missing+all-verity (clean pass)",
			images:    []string{"ghcr.io/verity-org/prometheus/prometheus:v3.9.1", "ghcr.io/verity-org/library/nginx:1.29.5-patched"},
			offenders: nil,
		},
		{
			name:      "missing+non-verity (hard fail)",
			images:    []string{"ghcr.io/verity-org/prometheus/prometheus:v3.9.1", "quay.io/upstream/foo:latest"},
			offenders: []string{"quay.io/upstream/foo:latest"},
		},
		{
			name:      "missing+empty namespace (clean pass — no images, no policy)",
			images:    nil,
			offenders: nil,
		},
		{
			name:      "missing+all-non-verity (hard fail, every image flagged)",
			images:    []string{"docker.io/library/postgres:17", "registry.k8s.io/sig-storage/csi-provisioner:v5.0.0"},
			offenders: []string{"docker.io/library/postgres:17", "registry.k8s.io/sig-storage/csi-provisioner:v5.0.0"},
		},
		{
			name:      "missing+double-ghcr-bug-pattern (chart-gen rewrite gap is non-verity)",
			images:    []string{"ghcr.io/ghcr.io/verity-org/foo:tag"},
			offenders: []string{"ghcr.io/ghcr.io/verity-org/foo:tag"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := classifyMissingAllowlist(c.images)
			if len(got) != len(c.offenders) {
				t.Fatalf("len got=%d want=%d (got=%v want=%v)", len(got), len(c.offenders), got, c.offenders)
			}
			for i := range c.offenders {
				if got[i] != c.offenders[i] {
					t.Fatalf("idx=%d got=%q want=%q", i, got[i], c.offenders[i])
				}
			}
		})
	}
}

// TestClassifyMissingAllowlistPresentMixed documents the policy
// boundary: classifyMissingAllowlist is only used when the
// allowlist file is *absent*. When present, AssertImageOrigin
// runs unchanged through isAccepted with a populated allow slice
// (covered by TestIsAccepted). This test asserts the helper's
// correct posture: it does NOT consult any allowlist, so a
// "present+mixed" caller would never reach it. We document that
// invariant here as a guard against future drift.
func TestClassifyMissingAllowlistDoesNotConsultAllowlist(t *testing.T) {
	// Even an image that WOULD be allowlisted under a
	// hypothetical present-allowlist test is still flagged here,
	// because classifyMissingAllowlist's whole point is "no
	// allowlist file exists, so non-verity images have no path
	// to acceptance."
	images := []string{"quay.io/special/escape-hatch:latest"}
	got := classifyMissingAllowlist(images)
	if len(got) != 1 || got[0] != images[0] {
		t.Fatalf("classifyMissingAllowlist must not consult an implicit allowlist; got %v", got)
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

func TestHarnessClusterCreatedFieldDefault(t *testing.T) {
	h := NewHarness(stdoutLogger{}, "/tmp")
	if h.clusterCreated {
		t.Fatal("new Harness must start with clusterCreated=false; otherwise Teardown could delete a pre-existing cluster the harness did not create")
	}
}

func TestInstallChartRejectsArgumentInjection(t *testing.T) {
	cases := []struct {
		name string
		spec config.ChartSpec
	}{
		{"name starts with dash", config.ChartSpec{Name: "-rf", Version: "1.0.0", Repository: "oci://example.com/charts"}},
		{"version starts with dash", config.ChartSpec{Name: "ok", Version: "--exec", Repository: "oci://example.com/charts"}},
		{"non-oci/http repo", config.ChartSpec{Name: "ok", Version: "1.0.0", Repository: "ssh://example.com/charts"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := InstallChart(context.Background(), &Harness{t: stdoutLogger{}}, c.spec, "")
			if err == nil {
				t.Fatalf("InstallChart should reject %v as argument-injection risk", c.spec)
			}
		})
	}
}
