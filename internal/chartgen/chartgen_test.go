package chartgen

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/verity-org/verity/internal/config"
)

func TestDryRunResultJSON(t *testing.T) {
	res := DryRunResult{
		Charts: []ChartResult{
			{
				Name:           "prometheus",
				Version:        "28.9.1",
				WrapperName:    "prometheus",
				WrapperVersion: "28.9.1",
				Registry:       "oci://ghcr.io/verity-org/charts",
				ImageMappings: []ImageMapping{
					{
						OriginalRepo: "quay.io/prometheus/prometheus",
						OriginalTag:  "v3.2.1",
						PatchedRepo:  "ghcr.io/verity-org/prometheus/prometheus",
						PatchedTag:   "v3.2.1",
					},
				},
				ValueOverrides: []ValueOverride{
					{
						Path:       "server.image",
						Repository: "ghcr.io/verity-org/prometheus/prometheus",
						Tag:        "v3.2.1",
					},
				},
			},
		},
	}

	b, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	charts, ok := doc["charts"].([]any)
	if !ok {
		t.Fatalf("charts type = %T, want []any", doc["charts"])
	}
	if len(charts) != 1 {
		t.Fatalf("charts length = %d, want 1", len(charts))
	}

	chart, ok := charts[0].(map[string]any)
	if !ok {
		t.Fatalf("charts[0] type = %T, want map[string]any", charts[0])
	}

	keys := []string{"name", "version", "wrapperName", "wrapperVersion", "registry", "imageMappings", "valueOverrides"}
	for _, key := range keys {
		if _, exists := chart[key]; !exists {
			t.Fatalf("chart missing key %q in JSON: %#v", key, chart)
		}
	}
}

func TestRunDryRunNoCharts(t *testing.T) {
	tmpDir := t.TempDir()

	chartsPath := filepath.Join(tmpDir, "does-not-exist-chart.yaml")
	verityPath := filepath.Join(tmpDir, "does-not-exist-verity.yaml")

	res, err := Run(&Config{
		ChartsFile:     chartsPath,
		VerityConfig:   verityPath,
		TargetRegistry: "ghcr.io/verity-org",
		ChartRegistry:  "oci://ghcr.io/verity-org/charts",
		ExcludeNames:   map[string]struct{}{},
		DryRun:         true,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if res == nil {
		t.Fatal("Run() result = nil, want non-nil")
	}
	if len(res.Charts) != 0 {
		t.Fatalf("Run() charts length = %d, want 0", len(res.Charts))
	}
}

func TestEnforceStrict(t *testing.T) {
	cases := []struct {
		name    string
		strict  bool
		total   int
		skipped int
		wantErr bool
	}{
		{name: "non-strict: 0 skipped is fine", strict: false, total: 5, skipped: 0, wantErr: false},
		{name: "non-strict: skipped tolerated", strict: false, total: 5, skipped: 3, wantErr: false},
		{name: "strict: 0 skipped is fine", strict: true, total: 5, skipped: 0, wantErr: false},
		{name: "strict: 1 skipped fails", strict: true, total: 5, skipped: 1, wantErr: true},
		{name: "strict: many skipped fails", strict: true, total: 5, skipped: 5, wantErr: true},
		{name: "non-strict, no charts: fine", strict: false, total: 0, skipped: 0, wantErr: false},
		{name: "strict, no charts: fine", strict: true, total: 0, skipped: 0, wantErr: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := enforceStrict(tc.strict, tc.total, tc.skipped, "/tmp/Chart.yaml")
			if (err != nil) != tc.wantErr {
				t.Errorf("enforceStrict(strict=%v, total=%d, skipped=%d) err=%v, wantErr=%v",
					tc.strict, tc.total, tc.skipped, err, tc.wantErr)
			}
			if tc.wantErr && err != nil {
				if !strings.Contains(err.Error(), "strict mode") {
					t.Errorf("error missing 'strict mode' prefix: %v", err)
				}
				if !strings.Contains(err.Error(), "Chart.yaml") {
					t.Errorf("error should reference charts file basename: %v", err)
				}
			}
		})
	}
}

func TestRunDryRunStrictNoChartsPasses(t *testing.T) {
	// With no charts loaded (empty Chart.yaml or missing file), strict
	// mode should NOT fail — there's nothing to skip. Guards against
	// over-eager error-on-empty.
	tmpDir := t.TempDir()
	res, err := Run(&Config{
		ChartsFile:     filepath.Join(tmpDir, "does-not-exist.yaml"),
		VerityConfig:   filepath.Join(tmpDir, "does-not-exist.yaml"),
		TargetRegistry: "ghcr.io/verity-org",
		ChartRegistry:  "oci://ghcr.io/verity-org/charts",
		ExcludeNames:   map[string]struct{}{},
		DryRun:         true,
		Strict:         true,
	})
	if err != nil {
		t.Fatalf("Run with strict=true and no charts: err=%v, want nil", err)
	}
	if res == nil || len(res.Charts) != 0 {
		t.Fatalf("expected 0 charts in result, got %v", res)
	}
}

func TestApplyReplacements(t *testing.T) {
	refs := []string{
		"registry.k8s.io/kube-state-metrics/kube-state-metrics:v2.18.0",
		"quay.io/prometheus/pushgateway:v1.11.2",
		"quay.io/prometheus/prometheus:v3.9.1",
	}

	vc := &config.VerityConfig{
		Replacements: map[string]config.Replacement{
			"kube-state-metrics/kube-state-metrics": {Registry: "ghcr.io/verity-org", Image: "kube-state-metrics"},
			"prometheus/pushgateway":                {Registry: "ghcr.io/verity-org", Image: "pushgateway", Tag: "1.11"},
		},
	}

	remaining, replacements := applyReplacements(refs, vc, nil)

	if len(remaining) != 1 {
		t.Fatalf("remaining = %d, want 1", len(remaining))
	}
	if remaining[0] != "quay.io/prometheus/prometheus:v3.9.1" {
		t.Errorf("remaining[0] = %q, want prometheus ref", remaining[0])
	}

	if len(replacements) != 2 {
		t.Fatalf("replacements = %d, want 2", len(replacements))
	}

	// kube-state-metrics: no Tag override, uses source tag
	ksm := replacements[0]
	if ksm.PatchedRepo != "ghcr.io/verity-org/kube-state-metrics" {
		t.Errorf("ksm PatchedRepo = %q", ksm.PatchedRepo)
	}
	if ksm.PatchedTag != "v2.18.0" {
		t.Errorf("ksm PatchedTag = %q, want v2.18.0", ksm.PatchedTag)
	}

	// pushgateway: Tag override
	pg := replacements[1]
	if pg.PatchedRepo != "ghcr.io/verity-org/pushgateway" {
		t.Errorf("pg PatchedRepo = %q", pg.PatchedRepo)
	}
	if pg.PatchedTag != "1.11" {
		t.Errorf("pg PatchedTag = %q, want 1.11", pg.PatchedTag)
	}
}

func TestApplyReplacementsNilConfig(t *testing.T) {
	refs := []string{"quay.io/foo/bar:v1.0"}
	remaining, replacements := applyReplacements(refs, nil, nil)
	if len(remaining) != 1 || len(replacements) != 0 {
		t.Fatalf("nil config: remaining=%d replacements=%d", len(remaining), len(replacements))
	}
}

func TestApplyReplacementsLongestPatternWins(t *testing.T) {
	// Regression test for substring-prefix collisions: a longer, more
	// specific pattern must win over a shorter pattern that is a substring
	// of the same image name. Lexicographic ordering would put the shorter
	// "kyverno/kyverno" before "kyverno/kyverno-cli" and incorrectly claim
	// the cli image; longest-first ordering fixes that.
	refs := []string{
		"reg.kyverno.io/kyverno/kyverno:v1.17.2",
		"reg.kyverno.io/kyverno/kyverno-cli:v1.17.2",
		"opensearchproject/opensearch:3.6.0",
		"opensearchproject/opensearch-dashboards:3.6.0",
	}
	vc := &config.VerityConfig{
		Replacements: map[string]config.Replacement{
			"kyverno/kyverno":                         {Registry: "ghcr.io/verity-org", Image: "kyverno"},
			"kyverno/kyverno-cli":                     {Registry: "ghcr.io/verity-org", Image: "kyverno-cli"},
			"opensearchproject/opensearch":            {Registry: "ghcr.io/verity-org", Image: "opensearch"},
			"opensearchproject/opensearch-dashboards": {Registry: "ghcr.io/verity-org", Image: "opensearch-dashboards"},
		},
	}

	_, replacements := applyReplacements(refs, vc, nil)

	want := map[string]string{
		"reg.kyverno.io/kyverno/kyverno":          "ghcr.io/verity-org/kyverno",
		"reg.kyverno.io/kyverno/kyverno-cli":      "ghcr.io/verity-org/kyverno-cli",
		"opensearchproject/opensearch":            "ghcr.io/verity-org/opensearch",
		"opensearchproject/opensearch-dashboards": "ghcr.io/verity-org/opensearch-dashboards",
	}
	if len(replacements) != len(want) {
		t.Fatalf("replacements = %d, want %d", len(replacements), len(want))
	}
	for _, r := range replacements {
		got := r.PatchedRepo
		expected, ok := want[r.OriginalRepo]
		if !ok {
			t.Errorf("unexpected OriginalRepo: %q", r.OriginalRepo)
			continue
		}
		if got != expected {
			t.Errorf("OriginalRepo=%q: got PatchedRepo=%q, want %q", r.OriginalRepo, got, expected)
		}
	}
}

func TestApplyReplacementsExcludedWithoutReplacement(t *testing.T) {
	// An image present in --exclude-names but NOT covered by any
	// replacement entry must be dropped (neither replaced nor passed
	// through to crane lookup).
	refs := []string{"quay.io/prometheus/pushgateway:v1.11.2"}
	vc := &config.VerityConfig{
		Replacements: map[string]config.Replacement{
			"some-unrelated/image": {Registry: "ghcr.io/verity-org", Image: "other"},
		},
	}
	exclude := map[string]struct{}{"pushgateway": {}}

	remaining, replacements := applyReplacements(refs, vc, exclude)
	if len(remaining) != 0 {
		t.Errorf("remaining = %d, want 0 (excluded)", len(remaining))
	}
	if len(replacements) != 0 {
		t.Errorf("replacements = %d, want 0 (no replacement matches, excluded)", len(replacements))
	}
}

func TestApplyReplacementsWinsOverExclude(t *testing.T) {
	// Regression test for the silent-failure case where chart-gen
	// produced zero wrappers for charts whose images had explicit
	// replacements but whose basenames also collided with Integer
	// rebuild filenames (the source of --exclude-names).
	//
	// Setup mirrors the real-world failure: an image whose basename
	// matches an exclude entry AND has an explicit replacement entry.
	// The replacement must win — otherwise the chart loses its mapping
	// and gets skipped with "no patched image mappings".
	refs := []string{
		"opensearchproject/opensearch:3.6.0",
		"registry.k8s.io/metrics-server/metrics-server:v0.8.0",
		"reg.kyverno.io/kyverno/kyverno:v1.17.2",
	}
	vc := &config.VerityConfig{
		Replacements: map[string]config.Replacement{
			"opensearchproject/opensearch":  {Registry: "ghcr.io/verity-org", Image: "opensearch"},
			"metrics-server/metrics-server": {Registry: "ghcr.io/verity-org", Image: "metrics-server"},
			"kyverno/kyverno":               {Registry: "ghcr.io/verity-org", Image: "kyverno"},
		},
	}
	// These exclude names match the basenames of the images above —
	// derived in the workflow from `find images -name '*.yaml'`.
	exclude := map[string]struct{}{
		"opensearch":     {},
		"metrics-server": {},
		"kyverno":        {},
	}

	remaining, replacements := applyReplacements(refs, vc, exclude)

	if len(remaining) != 0 {
		t.Errorf("remaining = %d, want 0 (all should be replaced); remaining=%v", len(remaining), remaining)
	}
	if len(replacements) != 3 {
		t.Fatalf("replacements = %d, want 3 (each image has an explicit replacement that must beat the exclude)", len(replacements))
	}

	want := map[string]string{
		"opensearchproject/opensearch":                  "ghcr.io/verity-org/opensearch",
		"registry.k8s.io/metrics-server/metrics-server": "ghcr.io/verity-org/metrics-server",
		"reg.kyverno.io/kyverno/kyverno":                "ghcr.io/verity-org/kyverno",
	}
	for _, r := range replacements {
		expected, ok := want[r.OriginalRepo]
		if !ok {
			t.Errorf("unexpected OriginalRepo: %q", r.OriginalRepo)
			continue
		}
		if r.PatchedRepo != expected {
			t.Errorf("OriginalRepo=%q: got PatchedRepo=%q, want %q (exclude must not block explicit replacement)", r.OriginalRepo, r.PatchedRepo, expected)
		}
	}
}
