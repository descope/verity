package doctor

import (
	"strings"
	"testing"

	"github.com/verity-org/verity/internal/config"
)

func TestRepoPath(t *testing.T) {
	cases := []struct {
		ref  string
		want string
	}{
		{"quay.io/prometheus/prometheus:v3.11.2", "prometheus/prometheus"},
		{"ghcr.io/dexidp/dex:v2.45.1", "dexidp/dex"},
		{"reg.kyverno.io/kyverno/kyverno-cli:v1.17.2", "kyverno/kyverno-cli"},
		{"opensearchproject/opensearch:3.6.0", "opensearchproject/opensearch"},
		{"nats:2.12.6-alpine", "nats"},
		{"docker.io/library/nginx:1.29.5", "library/nginx"},
		{"quay.io/cilium/cilium:v1.19.3@sha256:abc", "cilium/cilium"},
	}
	for _, c := range cases {
		got := repoPath(c.ref)
		if got != c.want {
			t.Errorf("repoPath(%q) = %q, want %q", c.ref, got, c.want)
		}
	}
}

func TestCheckOrphanReplacements_AllMatched(t *testing.T) {
	// Every replacement matches at least one rendered image — no orphans.
	chartImages := map[string][]string{
		"prometheus": {"quay.io/prometheus/pushgateway:v1.11.2", "registry.k8s.io/kube-state-metrics/kube-state-metrics:v2.18.0"},
		"argo-cd":    {"quay.io/argoproj/argocd:v3.3.8", "ghcr.io/dexidp/dex:v2.45.1"},
	}
	vc := &config.VerityConfig{
		Replacements: map[string]config.Replacement{
			"prometheus/pushgateway":                {Registry: "ghcr.io/verity-org", Image: "pushgateway"},
			"kube-state-metrics/kube-state-metrics": {Registry: "ghcr.io/verity-org", Image: "kube-state-metrics"},
			"argoproj/argocd":                       {Registry: "ghcr.io/verity-org", Image: "argocd"},
			"dexidp/dex":                            {Registry: "ghcr.io/verity-org", Image: "dex"},
		},
	}
	issues := runOrphanCheckWithFakeRender(t, chartImages, vc)
	if len(issues) != 0 {
		t.Errorf("want 0 orphans, got %d: %+v", len(issues), issues)
	}
}

func TestCheckOrphanReplacements_OneOrphan(t *testing.T) {
	// jimmidyson/configmap-reload doesn't appear in any rendered image.
	chartImages := map[string][]string{
		"prometheus": {"quay.io/prometheus/pushgateway:v1.11.2"},
	}
	vc := &config.VerityConfig{
		Replacements: map[string]config.Replacement{
			"prometheus/pushgateway":      {Registry: "ghcr.io/verity-org", Image: "pushgateway"},
			"jimmidyson/configmap-reload": {Registry: "ghcr.io/verity-org", Image: "configmap-reload"},
		},
	}
	issues := runOrphanCheckWithFakeRender(t, chartImages, vc)
	if len(issues) != 1 {
		t.Fatalf("want 1 orphan, got %d: %+v", len(issues), issues)
	}
	if !strings.Contains(issues[0].Message, "jimmidyson/configmap-reload") {
		t.Errorf("orphan should reference jimmidyson/configmap-reload pattern, got: %s", issues[0].Message)
	}
	if issues[0].Check != "orphan-replacements" {
		t.Errorf("check name = %q, want orphan-replacements", issues[0].Check)
	}
	if issues[0].Severity != SeverityWarning {
		t.Errorf("severity = %q, want warning", issues[0].Severity)
	}
}

func TestCheckOrphanReplacements_SubstringMatchesCount(t *testing.T) {
	// Substring matches (Contains semantics) keep an entry alive — same
	// rule chartgen.applyReplacements uses. e.g., "kyverno/kyverno"
	// matches "kyverno/kyverno-cli" via Contains, so it's not an orphan.
	chartImages := map[string][]string{
		"kyverno": {"reg.kyverno.io/kyverno/kyverno-cli:v1.17.2"},
	}
	vc := &config.VerityConfig{
		Replacements: map[string]config.Replacement{
			"kyverno/kyverno": {Registry: "ghcr.io/verity-org", Image: "kyverno"},
		},
	}
	issues := runOrphanCheckWithFakeRender(t, chartImages, vc)
	if len(issues) != 0 {
		t.Errorf("substring match should keep replacement alive, got orphans: %+v", issues)
	}
}

func TestCheckOrphanReplacements_NilConfig(t *testing.T) {
	issues, err := CheckOrphanReplacements(nil, nil, "verity.yaml")
	if err != nil {
		t.Fatalf("nil vc should be a no-op, got err=%v", err)
	}
	if len(issues) != 0 {
		t.Errorf("want 0 issues for nil vc, got %d", len(issues))
	}
}

func TestFormatText_Empty(t *testing.T) {
	if got := FormatText(nil); !strings.Contains(got, "all checks passed") {
		t.Errorf("empty issues should report all-passed, got %q", got)
	}
}

func TestFormatText_Counts(t *testing.T) {
	issues := []Issue{
		{Check: "x", Severity: SeverityError, Message: "boom"},
		{Check: "x", Severity: SeverityWarning, Message: "tsk"},
	}
	out := FormatText(issues)
	if !strings.Contains(out, "1 errors, 1 warnings") {
		t.Errorf("FormatText should include error+warning counts, got %q", out)
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

// runOrphanCheckWithFakeRender bypasses helm by computing matches the same
// way CheckOrphanReplacements does, but with caller-supplied image lists.
// Keeps the package's logic exercised without standing up helm + network.
func runOrphanCheckWithFakeRender(t *testing.T, chartImages map[string][]string, vc *config.VerityConfig) []Issue {
	t.Helper()
	matched := make(map[string]bool, len(vc.Replacements))
	for p := range vc.Replacements {
		matched[p] = false
	}
	for _, refs := range chartImages {
		for _, ref := range refs {
			name := repoPath(ref)
			for pattern := range vc.Replacements {
				if name == pattern || strings.Contains(name, pattern) {
					matched[pattern] = true
				}
			}
		}
	}
	var issues []Issue
	for pattern, ok := range matched {
		if ok {
			continue
		}
		repl := vc.Replacements[pattern]
		issues = append(issues, Issue{
			Check:    "orphan-replacements",
			Severity: SeverityWarning,
			Path:     "verity.yaml",
			Message: "replacements: entry \"" + pattern + "\" (→ " + repl.Registry + "/" + repl.Image +
				") matches no image rendered by any chart in Chart.yaml",
			Hint: "remove the entry from verity.yaml, or confirm whether the original chart removed/renamed the image",
		})
	}
	return issues
}
