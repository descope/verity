//go:build integration

package integration

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// validSkipsYAML is the smallest valid document used as the baseline; tests
// mutate it to cover each invariant.
const validSkipsYAML = `skips:
  - chart: falco
    reason: kernel-incompat
    tracking_issue: https://github.com/verity-org/verity/issues/325
    exit_criteria: "kernel >=6.x on GHA runners"
    added: 2026-05-14
    added_by: SCR-2026-05-14-001
  - chart: nfs-subdir-external-provisioner
    reason: needs-external-infra
    tracking_issue: "needs new issue"
    exit_criteria: "harness gains sidecar NFS server"
    added: 2026-05-14
    added_by: SCR-2026-05-14-001
`

func writeSkips(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "SKIPS.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write skips: %v", err)
	}
	return path
}

func TestLoadSkipsValid(t *testing.T) {
	cfg, err := LoadSkips(writeSkips(t, validSkipsYAML))
	if err != nil {
		t.Fatalf("LoadSkips: unexpected error: %v", err)
	}
	if len(cfg.Skips) != 2 {
		t.Fatalf("len(Skips)=%d want 2", len(cfg.Skips))
	}
	if cfg.Skips[0].Chart != "falco" {
		t.Errorf("Skips[0].Chart=%q want %q", cfg.Skips[0].Chart, "falco")
	}
	if cfg.Skips[0].TrackingIssue != "https://github.com/verity-org/verity/issues/325" {
		t.Errorf("falco tracking_issue=%q", cfg.Skips[0].TrackingIssue)
	}
	if cfg.Skips[1].TrackingIssue != "needs new issue" {
		t.Errorf("nfs tracking_issue=%q want sentinel", cfg.Skips[1].TrackingIssue)
	}
}

func TestLoadSkipsMissingFile(t *testing.T) {
	// Non-existent file = empty config, no error. Clean repo state.
	cfg, err := LoadSkips(filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if err != nil {
		t.Fatalf("LoadSkips(missing): unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config for missing file")
	}
	if len(cfg.Skips) != 0 {
		t.Errorf("expected empty Skips, got %d", len(cfg.Skips))
	}
	if got, _ := cfg.IsSkipped("anything"); got {
		t.Error("IsSkipped on empty config returned true")
	}
}

func TestLoadSkipsMalformedYAML(t *testing.T) {
	// Tabs inside YAML mapping values + missing closing bracket = parse error.
	bad := "skips:\n  - chart: falco\n\treason: oops [\n"
	_, err := LoadSkips(writeSkips(t, bad))
	if err == nil {
		t.Fatal("expected parse error for malformed YAML, got nil")
	}
	if !strings.Contains(err.Error(), "parse") {
		t.Errorf("error should mention parse, got: %v", err)
	}
}

func TestLoadSkipsUnknownField(t *testing.T) {
	// KnownFields(true) means a typo'd top-level key is rejected — a useful
	// guardrail against e.g. `skip:` (singular) silently producing empty config.
	bad := `skip:
  - chart: falco
    reason: x
    tracking_issue: "needs new issue"
    exit_criteria: y
    added: 2026-05-14
    added_by: me
`
	_, err := LoadSkips(writeSkips(t, bad))
	if err == nil {
		t.Fatal("expected error for unknown top-level field, got nil")
	}
}

func TestLoadSkipsDuplicateChart(t *testing.T) {
	body := `skips:
  - chart: falco
    reason: a
    tracking_issue: "needs new issue"
    exit_criteria: x
    added: 2026-05-14
    added_by: me
  - chart: falco
    reason: b
    tracking_issue: "needs new issue"
    exit_criteria: y
    added: 2026-05-14
    added_by: me
`
	_, err := LoadSkips(writeSkips(t, body))
	if err == nil {
		t.Fatal("expected error for duplicate chart")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("error should mention duplicate, got: %v", err)
	}
}

func TestLoadSkipsExceedsCap(t *testing.T) {
	if MaxSkippedCharts != 5 {
		t.Fatalf("test assumes MaxSkippedCharts=5, got %d — update test", MaxSkippedCharts)
	}
	var b strings.Builder
	b.WriteString("skips:\n")
	for i := range MaxSkippedCharts + 1 {
		// chart-N-of-M form keeps names unique so the duplicate check
		// can't accidentally fire first.
		_, _ = b.WriteString("  - chart: chart" + string(rune('a'+i)) + "\n")
		b.WriteString("    reason: x\n")
		b.WriteString("    tracking_issue: \"needs new issue\"\n")
		b.WriteString("    exit_criteria: y\n")
		b.WriteString("    added: 2026-05-14\n")
		b.WriteString("    added_by: me\n")
	}
	_, err := LoadSkips(writeSkips(t, b.String()))
	if err == nil {
		t.Fatal("expected error for >MaxSkippedCharts entries")
	}
	if !strings.Contains(err.Error(), "hard cap") {
		t.Errorf("error should mention hard cap, got: %v", err)
	}
}

func TestLoadSkipsMissingRequiredField(t *testing.T) {
	// Each row of the table omits exactly one required field.
	base := map[string]string{
		"chart":          "falco",
		"reason":         "x",
		"tracking_issue": `"needs new issue"`,
		"exit_criteria":  "y",
		"added":          "2026-05-14",
		"added_by":       "me",
	}
	required := []string{"chart", "reason", "tracking_issue", "exit_criteria", "added", "added_by"}

	for _, omit := range required {
		t.Run("omit_"+omit, func(t *testing.T) {
			var b strings.Builder
			b.WriteString("skips:\n  - ")
			first := true
			for _, k := range required {
				if k == omit {
					continue
				}
				if first {
					b.WriteString(k + ": " + base[k] + "\n")
					first = false
				} else {
					b.WriteString("    " + k + ": " + base[k] + "\n")
				}
			}
			_, err := LoadSkips(writeSkips(t, b.String()))
			if err == nil {
				t.Fatalf("expected error when %q omitted, got nil", omit)
			}
			if !strings.Contains(err.Error(), omit) {
				t.Errorf("error should mention missing field %q, got: %v", omit, err)
			}
		})
	}
}

func TestLoadSkipsUnsafeChartName(t *testing.T) {
	cases := map[string]string{
		"slash":      "foo/bar",
		"backslash":  `foo\bar`,
		"dotdot":     "../etc",
		"whitespace": "foo bar",
		"newline":    "foo\nbar",
	}
	for label, badName := range cases {
		t.Run(label, func(t *testing.T) {
			body := "skips:\n  - chart: " + yamlQuote(badName) + "\n" +
				"    reason: x\n" +
				"    tracking_issue: \"needs new issue\"\n" +
				"    exit_criteria: y\n" +
				"    added: 2026-05-14\n" +
				"    added_by: me\n"
			_, err := LoadSkips(writeSkips(t, body))
			if err == nil {
				t.Fatalf("expected error for unsafe chart name %q", badName)
			}
		})
	}
}

func TestLoadSkipsBadTrackingIssue(t *testing.T) {
	// Anything that is neither the sentinel nor a github.com http(s) URL
	// must be rejected. Three failure cases keep coverage tight.
	cases := []string{
		"https://gitlab.com/foo/bar/issues/1", // wrong host
		"github.com/foo/bar/issues/1",         // missing scheme
		"TODO",                                // free-text junk
	}
	for _, bad := range cases {
		t.Run(bad, func(t *testing.T) {
			body := "skips:\n  - chart: falco\n" +
				"    reason: x\n" +
				"    tracking_issue: " + yamlQuote(bad) + "\n" +
				"    exit_criteria: y\n" +
				"    added: 2026-05-14\n" +
				"    added_by: me\n"
			_, err := LoadSkips(writeSkips(t, body))
			if err == nil {
				t.Fatalf("expected error for tracking_issue=%q", bad)
			}
			if !strings.Contains(err.Error(), "tracking_issue") {
				t.Errorf("error should mention tracking_issue, got: %v", err)
			}
		})
	}
}

func TestIsSkippedMatchAndMiss(t *testing.T) {
	cfg, err := LoadSkips(writeSkips(t, validSkipsYAML))
	if err != nil {
		t.Fatalf("LoadSkips: %v", err)
	}

	hit, entry := cfg.IsSkipped("falco")
	if !hit {
		t.Fatal("IsSkipped(falco) returned false, want true")
	}
	if entry == nil {
		t.Fatal("IsSkipped(falco) returned nil entry")
	}
	if entry.Reason != "kernel-incompat" {
		t.Errorf("entry.Reason=%q want kernel-incompat", entry.Reason)
	}
	if entry.TrackingIssue != "https://github.com/verity-org/verity/issues/325" {
		t.Errorf("entry.TrackingIssue=%q", entry.TrackingIssue)
	}

	hit2, entry2 := cfg.IsSkipped("argo-cd")
	if hit2 {
		t.Errorf("IsSkipped(argo-cd) returned true, want false")
	}
	if entry2 != nil {
		t.Error("expected nil entry on miss")
	}
}

func TestIsSkippedNilSafe(t *testing.T) {
	// A nil *SkipsConfig must not panic — defensive guard for callers that
	// forgot to call LoadSkips. Mirrors the empty-config invariant.
	var cfg *SkipsConfig
	hit, entry := cfg.IsSkipped("falco")
	if hit || entry != nil {
		t.Error("nil receiver should yield (false, nil)")
	}
}

// yamlQuote wraps a string in YAML double quotes, escaping inner double
// quotes and backslashes so test fixtures with special characters round-trip
// through yaml.v3 unchanged.
func yamlQuote(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	return `"` + s + `"`
}

// Sanity: the production SKIPS.yaml in the repo must itself load cleanly.
// This catches a CI-only foot-gun where someone edits SKIPS.yaml without
// running the unit tests locally.
func TestProductionSKIPSYAMLIsValid(t *testing.T) {
	root, err := findRepoRoot()
	if err != nil {
		t.Skipf("findRepoRoot failed (likely not running in repo): %v", err)
	}
	path := filepath.Join(root, "test", "chart-integration", "SKIPS.yaml")
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			t.Skip("SKIPS.yaml not present in repo; skipping production validation")
		}
		t.Fatalf("stat SKIPS.yaml: %v", err)
	}
	cfg, err := LoadSkips(path)
	if err != nil {
		t.Fatalf("production SKIPS.yaml failed validation: %v", err)
	}
	if len(cfg.Skips) > MaxSkippedCharts {
		t.Fatalf("production SKIPS.yaml has %d entries, cap is %d", len(cfg.Skips), MaxSkippedCharts)
	}
}
