// Package doctor lints Verity's configuration cross-references for known
// silent-failure patterns. Today it ships one check
// (CheckOrphanReplacements); the package is structured so additional checks
// can be added incrementally as separate Check* functions.
package doctor

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/verity-org/verity/internal/config"
	"github.com/verity-org/verity/internal/discovery"
	"github.com/verity-org/verity/internal/imageref"
)

// Severity classifies a doctor finding. error fails the run; warning is
// informational unless --fail-on-warning is set by the caller.
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

// Config controls a doctor run. Empty paths default to the repo-root file
// names; a missing file is an error rather than a silent no-op (otherwise a
// typoed --charts-file would report every replacement as orphan).
type Config struct {
	ChartsFile   string
	VerityConfig string
}

// ErrChartsFileMissing is returned by Run when the configured charts file
// does not exist. Callers can errors.Is-check it to distinguish "config
// gone" from a check that produced findings.
var ErrChartsFileMissing = errors.New("charts file does not exist")

// ChartImagesFunc renders a chart and returns the image references it
// emits. Production code passes discovery.ExtractChartImages; tests pass a
// stub so CheckOrphanReplacements can be exercised without helm + network.
type ChartImagesFunc func(chart config.ChartSpec, overrides map[string]config.Override) ([]string, error)

// Run executes all checks and returns the aggregated findings sorted by
// (check, severity, message) for stable output. The returned slice is never
// nil — Run returns []Issue{} when no findings are produced so JSON
// callers see an array, not null.
func Run(cfg Config) ([]Issue, error) {
	if cfg.ChartsFile == "" {
		cfg.ChartsFile = "Chart.yaml"
	}
	if cfg.VerityConfig == "" {
		cfg.VerityConfig = "verity.yaml"
	}

	if _, err := os.Stat(cfg.ChartsFile); errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%w: %s", ErrChartsFileMissing, cfg.ChartsFile)
	}

	charts, err := discovery.LoadChartsFile(cfg.ChartsFile)
	if err != nil {
		return nil, fmt.Errorf("load charts file %s: %w", cfg.ChartsFile, err)
	}

	vc, err := discovery.LoadVerityConfig(cfg.VerityConfig)
	if err != nil {
		return nil, fmt.Errorf("load verity config %s: %w", cfg.VerityConfig, err)
	}

	chartImages := func(chart config.ChartSpec, overrides map[string]config.Override) ([]string, error) {
		return discovery.ExtractChartImagesWithValues(chart, overrides, vc.ChartValues[chart.Name])
	}
	orphans, err := CheckOrphanReplacements(charts, vc, cfg.VerityConfig, cfg.ChartsFile, chartImages)
	if err != nil {
		return nil, fmt.Errorf("orphan-replacements check: %w", err)
	}

	// Pre-allocated to len(orphans) so the slice header is non-nil even
	// when there are zero issues; downstream JSON output then renders as
	// [] instead of null.
	issues := make([]Issue, 0, len(orphans))
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
// — once it wired a real chart image to an Integer rebuild, but the chart
// upgraded or the image moved and the entry was never cleaned up.
//
// The chartImages parameter abstracts the helm-shelling step so production
// code passes discovery.ExtractChartImages while tests pass a stub. This
// lets the same function be exercised by fast unit tests without requiring
// helm + network.
//
// Match semantics intentionally mirror chartgen.applyReplacements (Contains
// after RepoPath) so this check reasons about images the same way the
// runtime matcher does. Note: chartgen also sorts patterns longest-first
// for first-match selection, which can mean a shorter pattern technically
// matches an image but a longer one is selected at runtime. For the orphan
// check, "matches anywhere" is the right semantic — the pattern still has
// a downstream consumer if any image's name contains it, even if a more
// specific pattern claims that image first at runtime.
func CheckOrphanReplacements(
	charts []config.ChartSpec,
	vc *config.VerityConfig,
	verityConfigPath, chartsFilePath string,
	chartImages ChartImagesFunc,
) ([]Issue, error) {
	if vc == nil || len(vc.Replacements) == 0 {
		return nil, nil
	}

	matched := make(map[string]bool, len(vc.Replacements))
	for pattern := range vc.Replacements {
		matched[pattern] = false
	}

	for _, chart := range charts {
		refs, err := chartImages(chart, vc.Overrides)
		if err != nil {
			return nil, fmt.Errorf("extract images for chart %s@%s: %w", chart.Name, chart.Version, err)
		}
		for _, ref := range refs {
			name := imageref.RepoPath(ref)
			for pattern := range vc.Replacements {
				if name == pattern || strings.Contains(name, pattern) {
					matched[pattern] = true
				}
			}
		}
	}

	issues := make([]Issue, 0, len(matched))
	for pattern, ok := range matched {
		if ok {
			continue
		}
		repl := vc.Replacements[pattern]
		issues = append(issues, Issue{
			Check:    "orphan-replacements",
			Severity: SeverityWarning,
			Path:     verityConfigPath,
			Message:  fmt.Sprintf("replacements: entry %q (→ %s/%s) matches no image rendered by any chart in %s", pattern, repl.Registry, repl.Image, chartsFilePath),
			Hint:     fmt.Sprintf("remove the entry from %s, or confirm whether the original chart removed/renamed the image", verityConfigPath),
		})
	}
	return issues, nil
}

// FormatText renders issues as a human-readable report. Empty input yields
// a single "all checks passed" line.
func FormatText(issues []Issue) string {
	if len(issues) == 0 {
		return "verity doctor: all checks passed.\n"
	}
	var b strings.Builder
	errs, warns := 0, 0
	for _, iss := range issues {
		fmt.Fprintf(&b, "[%s] %s: %s\n", iss.Severity, iss.Check, iss.Message)
		if iss.Path != "" {
			fmt.Fprintf(&b, "  in: %s\n", iss.Path)
		}
		if iss.Hint != "" {
			fmt.Fprintf(&b, "  hint: %s\n", iss.Hint)
		}
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
