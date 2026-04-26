// Package doctor lints Verity's configuration cross-references — Chart.yaml,
// verity.yaml, copa-config.yaml, and the images/ Integer rebuilds — to catch
// silent failure modes (orphan replacements, charts with no patched
// mappings, dangling Integer rebuilds, etc.) before they reach production.
//
// Each check is a small function that returns Issues. Run aggregates and
// formats them. Today the package ships a single check
// (CheckOrphanReplacements); future PRs add more.
package doctor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/verity-org/verity/internal/config"
	"github.com/verity-org/verity/internal/discovery"
)

// Severity classifies a doctor finding. error fails the run; warning is
// informational.
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
)

// Issue is a single linter finding.
type Issue struct {
	Check    string   `json:"check"`
	Severity Severity `json:"severity"`
	Path     string   `json:"path,omitempty"`
	Message  string   `json:"message"`
	Hint     string   `json:"hint,omitempty"`
}

// Config controls a doctor run. Paths default to the repo-root file names
// when empty.
type Config struct {
	ChartsFile   string
	VerityConfig string
}

// Run executes all checks and returns the aggregated findings sorted by
// (check, severity, message) for stable output.
func Run(cfg Config) ([]Issue, error) {
	if cfg.ChartsFile == "" {
		cfg.ChartsFile = "Chart.yaml"
	}
	if cfg.VerityConfig == "" {
		cfg.VerityConfig = "verity.yaml"
	}

	charts, err := discovery.LoadChartsFile(cfg.ChartsFile)
	if err != nil {
		return nil, fmt.Errorf("load charts file %s: %w", cfg.ChartsFile, err)
	}

	vc, err := discovery.LoadVerityConfig(cfg.VerityConfig)
	if err != nil {
		return nil, fmt.Errorf("load verity config %s: %w", cfg.VerityConfig, err)
	}

	var issues []Issue

	orphans, err := CheckOrphanReplacements(charts, vc, cfg.VerityConfig)
	if err != nil {
		return nil, fmt.Errorf("orphan-replacements check: %w", err)
	}
	issues = append(issues, orphans...)

	sort.Slice(issues, func(i, j int) bool {
		if issues[i].Check != issues[j].Check {
			return issues[i].Check < issues[j].Check
		}
		if issues[i].Severity != issues[j].Severity {
			return issues[i].Severity < issues[j].Severity
		}
		return issues[i].Message < issues[j].Message
	})

	return issues, nil
}

// CheckOrphanReplacements finds entries in vc.Replacements that match no
// image rendered by any chart in charts. An orphan is a stale config entry
// — once it was wiring a real chart image to an Integer rebuild, but the
// chart upgraded or the image moved and the entry was never cleaned up.
//
// This check shells out to `helm template` (via discovery.ExtractChartImages)
// once per chart, so it requires helm in PATH and network access to chart
// repositories. In CI the existing chart-gen workflow already exercises that
// path; running doctor in the same job is cheap.
func CheckOrphanReplacements(charts []config.ChartSpec, vc *config.VerityConfig, verityConfigPath string) ([]Issue, error) {
	if vc == nil || len(vc.Replacements) == 0 {
		return nil, nil
	}

	matchedPatterns := make(map[string]bool, len(vc.Replacements))
	for pattern := range vc.Replacements {
		matchedPatterns[pattern] = false
	}

	for _, chart := range charts {
		refs, err := discovery.ExtractChartImages(chart, vc.Overrides)
		if err != nil {
			return nil, fmt.Errorf("extract images for chart %s@%s: %w", chart.Name, chart.Version, err)
		}
		for _, ref := range refs {
			name := repoPath(ref)
			for pattern := range vc.Replacements {
				if name == pattern || strings.Contains(name, pattern) {
					matchedPatterns[pattern] = true
				}
			}
		}
	}

	var issues []Issue
	for pattern, matched := range matchedPatterns {
		if matched {
			continue
		}
		repl := vc.Replacements[pattern]
		issues = append(issues, Issue{
			Check:    "orphan-replacements",
			Severity: SeverityWarning,
			Path:     verityConfigPath,
			Message:  fmt.Sprintf("replacements: entry %q (→ %s/%s) matches no image rendered by any chart in Chart.yaml", pattern, repl.Registry, repl.Image),
			Hint:     "remove the entry from verity.yaml, or confirm whether the original chart removed/renamed the image",
		})
	}
	return issues, nil
}

// repoPath strips the registry host (if it has a dot/colon, signaling a
// hostname rather than a Docker Hub library namespace) and any tag/digest,
// leaving just the path used for replacements: matching. Mirrors the
// matcher in internal/chartgen so doctor reasons about images the same way
// chart-gen does.
func repoPath(ref string) string {
	if idx := strings.Index(ref, "@"); idx != -1 {
		ref = ref[:idx]
	}
	lastSlash := strings.LastIndex(ref, "/")
	if lastColon := strings.LastIndex(ref, ":"); lastColon > lastSlash {
		ref = ref[:lastColon]
	}
	parts := strings.Split(ref, "/")
	if len(parts) >= 2 {
		first := parts[0]
		if strings.ContainsAny(first, ".:") || first == "localhost" {
			return strings.Join(parts[1:], "/")
		}
	}
	return ref
}

// FormatText renders issues as a human-readable report. Empty input yields
// a single "all checks passed" line.
func FormatText(issues []Issue) string {
	if len(issues) == 0 {
		return "verity doctor: all checks passed.\n"
	}
	var b strings.Builder
	for _, iss := range issues {
		fmt.Fprintf(&b, "[%s] %s: %s\n", iss.Severity, iss.Check, iss.Message)
		if iss.Path != "" {
			fmt.Fprintf(&b, "  in: %s\n", iss.Path)
		}
		if iss.Hint != "" {
			fmt.Fprintf(&b, "  hint: %s\n", iss.Hint)
		}
	}
	errs := 0
	warns := 0
	for _, iss := range issues {
		switch iss.Severity {
		case SeverityError:
			errs++
		case SeverityWarning:
			warns++
		}
	}
	fmt.Fprintf(&b, "verity doctor: %d issues (%d errors, %d warnings)\n", len(issues), errs, warns)
	return b.String()
}

// HasErrors reports whether any issue has Severity error.
func HasErrors(issues []Issue) bool {
	for _, iss := range issues {
		if iss.Severity == SeverityError {
			return true
		}
	}
	return false
}
