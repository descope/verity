package doctor

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/verity-org/verity/internal/config"
)

// fakeExtractor returns canned image lists keyed by chart name. Callers
// supply the map; chart specs that aren't in the map render no images.
func fakeExtractor(byChart map[string][]string) ChartImagesFunc {
	return func(chart config.ChartSpec, _ map[string]config.Override) ([]string, error) {
		return byChart[chart.Name], nil
	}
}

func TestCheckOrphanReplacements_AllMatched(t *testing.T) {
	charts := []config.ChartSpec{
		{Name: "prometheus", Version: "29.2.1"},
		{Name: "argo-cd", Version: "9.5.4"},
	}
	chartImages := map[string][]string{
		"prometheus": {
			"quay.io/prometheus/pushgateway:v1.11.2",
			"registry.k8s.io/kube-state-metrics/kube-state-metrics:v2.18.0",
		},
		"argo-cd": {"quay.io/argoproj/argocd:v3.3.8", "ghcr.io/dexidp/dex:v2.45.1"},
	}
	vc := &config.VerityConfig{
		Replacements: map[string]config.Replacement{
			"prometheus/pushgateway":                {Registry: "ghcr.io/verity-org", Image: "pushgateway"},
			"kube-state-metrics/kube-state-metrics": {Registry: "ghcr.io/verity-org", Image: "kube-state-metrics"},
			"argoproj/argocd":                       {Registry: "ghcr.io/verity-org", Image: "argocd"},
			"dexidp/dex":                            {Registry: "ghcr.io/verity-org", Image: "dex"},
		},
	}

	issues, err := CheckOrphanReplacements(charts, vc, "verity.yaml", "Chart.yaml", fakeExtractor(chartImages))
	if err != nil {
		t.Fatalf("CheckOrphanReplacements err = %v", err)
	}
	if len(issues) != 0 {
		t.Errorf("want 0 orphans, got %d: %+v", len(issues), issues)
	}
}

func TestCheckOrphanReplacements_OneOrphan(t *testing.T) {
	charts := []config.ChartSpec{{Name: "prometheus", Version: "29.2.1"}}
	chartImages := map[string][]string{
		"prometheus": {"quay.io/prometheus/pushgateway:v1.11.2"},
	}
	vc := &config.VerityConfig{
		Replacements: map[string]config.Replacement{
			"prometheus/pushgateway":      {Registry: "ghcr.io/verity-org", Image: "pushgateway"},
			"jimmidyson/configmap-reload": {Registry: "ghcr.io/verity-org", Image: "configmap-reload"},
		},
	}

	issues, err := CheckOrphanReplacements(charts, vc, "verity.yaml", "Chart.yaml", fakeExtractor(chartImages))
	if err != nil {
		t.Fatalf("CheckOrphanReplacements err = %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("want 1 orphan, got %d: %+v", len(issues), issues)
	}
	got := issues[0]
	if got.Check != "orphan-replacements" {
		t.Errorf("check = %q, want orphan-replacements", got.Check)
	}
	if got.Severity != SeverityWarning {
		t.Errorf("severity = %q, want warning", got.Severity)
	}
	if !strings.Contains(got.Message, "jimmidyson/configmap-reload") {
		t.Errorf("message should reference orphan pattern: %s", got.Message)
	}
	if !strings.Contains(got.Message, "Chart.yaml") {
		t.Errorf("message should reference chartsFilePath: %s", got.Message)
	}
	if !strings.Contains(got.Hint, "verity.yaml") {
		t.Errorf("hint should reference verityConfigPath: %s", got.Hint)
	}
}

func TestCheckOrphanReplacements_SubstringMatchesCount(t *testing.T) {
	charts := []config.ChartSpec{{Name: "kyverno", Version: "3.7.2"}}
	chartImages := map[string][]string{
		"kyverno": {"reg.kyverno.io/kyverno/kyverno-cli:v1.17.2"},
	}
	vc := &config.VerityConfig{
		Replacements: map[string]config.Replacement{
			"kyverno/kyverno": {Registry: "ghcr.io/verity-org", Image: "kyverno"},
		},
	}
	issues, err := CheckOrphanReplacements(charts, vc, "verity.yaml", "Chart.yaml", fakeExtractor(chartImages))
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(issues) != 0 {
		t.Errorf("substring match should keep replacement alive, got orphans: %+v", issues)
	}
}

func TestCheckOrphanReplacements_NilConfig(t *testing.T) {
	issues, err := CheckOrphanReplacements(nil, nil, "verity.yaml", "Chart.yaml", fakeExtractor(nil))
	if err != nil {
		t.Fatalf("nil vc should be a no-op, got err=%v", err)
	}
	if len(issues) != 0 {
		t.Errorf("want 0 issues for nil vc, got %d", len(issues))
	}
}

var errBoom = errors.New("boom")

func TestCheckOrphanReplacements_ExtractorError(t *testing.T) {
	charts := []config.ChartSpec{{Name: "broken", Version: "0.0.0"}}
	vc := &config.VerityConfig{
		Replacements: map[string]config.Replacement{"x": {Registry: "r", Image: "i"}},
	}
	failingExtractor := func(_ config.ChartSpec, _ map[string]config.Override) ([]string, error) {
		return nil, errBoom
	}
	_, err := CheckOrphanReplacements(charts, vc, "verity.yaml", "Chart.yaml", failingExtractor)
	if err == nil {
		t.Fatal("expected error to surface from extractor")
	}
	if !errors.Is(err, errBoom) {
		t.Errorf("error chain should include extractor error, got: %v", err)
	}
}

func TestRun_MissingChartsFile(t *testing.T) {
	tmp := t.TempDir()
	_, err := Run(Config{
		ChartsFile:   filepath.Join(tmp, "no-such.yaml"),
		VerityConfig: filepath.Join(tmp, "no-such-verity.yaml"),
	})
	if err == nil {
		t.Fatal("expected ErrChartsFileMissing for nonexistent file")
	}
	if !errors.Is(err, ErrChartsFileMissing) {
		t.Errorf("error should wrap ErrChartsFileMissing, got: %v", err)
	}
}

func TestRun_NeverNilSlice(t *testing.T) {
	// JSON callers expect an array, not null. Verify the empty-result
	// path returns a non-nil slice even though it's empty.
	tmp := t.TempDir()
	chartsPath := filepath.Join(tmp, "Chart.yaml")
	verityPath := filepath.Join(tmp, "verity.yaml")
	if err := os.WriteFile(chartsPath, []byte("apiVersion: v2\nname: test\nversion: 0.0.0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(verityPath, []byte("replacements: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	issues, err := Run(Config{ChartsFile: chartsPath, VerityConfig: verityPath})
	if err != nil {
		t.Fatalf("Run err = %v", err)
	}
	if issues == nil {
		t.Error("Run should return non-nil empty slice, got nil")
	}
	if len(issues) != 0 {
		t.Errorf("want 0 issues for empty config, got %d", len(issues))
	}
}

func TestFormatText_Empty(t *testing.T) {
	if got := FormatText(nil); !strings.Contains(got, "all checks passed") {
		t.Errorf("empty issues should report all-passed, got %q", got)
	}
}

func TestFormatText_Counts(t *testing.T) {
	issues := []Issue{
		{Check: "x", Severity: SeverityError, Message: "boom", Path: "a.yaml", Hint: "fix it"},
		{Check: "x", Severity: SeverityWarning, Message: "tsk"},
	}
	out := FormatText(issues)
	for _, want := range []string{"1 errors, 1 warnings", "boom", "tsk", "fix it", "a.yaml"} {
		if !strings.Contains(out, want) {
			t.Errorf("FormatText missing %q in output: %q", want, out)
		}
	}
}

func TestHasErrors(t *testing.T) {
	if HasErrors(nil) {
		t.Error("HasErrors(nil) should be false")
	}
	if HasErrors([]Issue{{Severity: SeverityWarning}}) {
		t.Error("warning-only issues should not count as errors")
	}
	if !HasErrors([]Issue{{Severity: SeverityWarning}, {Severity: SeverityError}}) {
		t.Error("at least one error should make HasErrors true")
	}
}
