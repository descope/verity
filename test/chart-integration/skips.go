//go:build integration

package integration

// SKIPS.yaml loader for the chart-integration smoke harness.
//
// Contract (SCR-2026-05-14-001, AC-1 + AC-3):
//
//   - The file is optional. If it does not exist, the loader returns an empty
//     config with no error (clean repo state = no skips).
//
//   - If the file IS present, it MUST parse cleanly and validate. Any
//     malformed YAML, missing required field, structural anomaly, duplicate
//     chart entry, or breach of the hard cap (MaxSkippedCharts) MUST cause
//     LoadSkips to return an error. The TestMain layer treats this error as
//     fatal — silent skips are the worst possible failure mode for a smoke
//     suite that exists to catch regressions.
//
//   - Required per-entry fields: chart, reason, tracking_issue,
//     exit_criteria, added, added_by.
//
//   - tracking_issue MUST be either the literal sentinel "needs new issue"
//     (so an audit script can detect un-issued skips) OR a URL containing
//     "github.com" (we keep this loose intentionally — full URL validation
//     belongs in the audit script, not the harness).
//
//   - chart names are validated for safe shape (no whitespace, no slashes,
//     no path traversal) because they flow into log messages and may, in
//     future, be used as path fragments in summary artifact filenames.

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// MaxSkippedCharts is the hard cap on entries in SKIPS.yaml, mirroring
// SCR-2026-05-14-001 AC-1 ("≤5 explicitly annotated expected-skip"). Hard-coded
// as a constant rather than read from config so the cap is visible in code
// review of any future increase.
const MaxSkippedCharts = 5

// skipsNeedsNewIssueSentinel is the only non-URL value accepted for
// tracking_issue. An audit script (subtask 9 / future) can grep for this
// string to enumerate un-issued skips.
const skipsNeedsNewIssueSentinel = "needs new issue"

// Static error sentinels. Defining them here (vs. inline errors.New /
// fmt.Errorf at call sites) keeps golangci-lint's err113 happy and lets
// tests errors.Is-match on intent if they ever need to.
var (
	errSkipsChartHasWhitespace    = errors.New("contains whitespace")
	errSkipsChartHasSlash         = errors.New("contains slash")
	errSkipsChartHasBackslash     = errors.New("contains backslash")
	errSkipsChartHasPathTraversal = errors.New("contains path-traversal sequence")
	errSkipsBadTrackingIssue      = fmt.Errorf(
		"tracking_issue must be %q OR a github.com http(s) URL",
		skipsNeedsNewIssueSentinel,
	)
	errSkipsCapExceeded     = errors.New("skip entries exceed hard cap (SCR-2026-05-14-001 AC-1) — either remove an entry or open an SCR to raise the cap")
	errSkipsDuplicateChart  = errors.New("duplicate skip entry for chart")
	errSkipsMissingRequired = errors.New("required field is empty")
)

// SkipEntry is one row of SKIPS.yaml. All fields are mandatory.
type SkipEntry struct {
	Chart         string `yaml:"chart"`
	Reason        string `yaml:"reason"`
	TrackingIssue string `yaml:"tracking_issue"`
	ExitCriteria  string `yaml:"exit_criteria"`
	Added         string `yaml:"added"`
	AddedBy       string `yaml:"added_by"`
}

// SkipsConfig is the top-level YAML document.
type SkipsConfig struct {
	Skips []SkipEntry `yaml:"skips"`

	// byChart is a lookup index built during validation. Never serialized.
	byChart map[string]*SkipEntry `yaml:"-"`
}

// LoadSkips reads and validates a SKIPS.yaml file.
//
// If path does not exist, returns an empty *SkipsConfig and a nil error —
// no skips configured is a valid state. Any other I/O error, parse error, or
// validation failure returns a descriptive error and a nil config; callers
// MUST treat this as fatal.
func LoadSkips(path string) (*SkipsConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &SkipsConfig{Skips: nil, byChart: map[string]*SkipEntry{}}, nil
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	cfg := &SkipsConfig{}
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true) // reject typo'd top-level keys
	if err := dec.Decode(cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	if err := cfg.validate(path); err != nil {
		return nil, err
	}
	return cfg, nil
}

// validate enforces the SKIPS.yaml contract: hard cap, required fields,
// uniqueness of chart names, safe chart names, and tracking_issue shape.
func (s *SkipsConfig) validate(path string) error {
	if len(s.Skips) > MaxSkippedCharts {
		return fmt.Errorf("%s: %w: have %d, cap is %d",
			path, errSkipsCapExceeded, len(s.Skips), MaxSkippedCharts)
	}

	s.byChart = make(map[string]*SkipEntry, len(s.Skips))
	for i := range s.Skips {
		e := &s.Skips[i]
		if err := validateSkipEntry(e, i); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if _, dup := s.byChart[e.Chart]; dup {
			return fmt.Errorf("%s: %w %q (index %d)", path, errSkipsDuplicateChart, e.Chart, i)
		}
		s.byChart[e.Chart] = e
	}
	return nil
}

// validateSkipEntry enforces per-entry invariants. The index is included in
// every error so a malformed file can be triaged without line numbers.
func validateSkipEntry(e *SkipEntry, idx int) error {
	required := []struct {
		name string
		val  string
	}{
		{"chart", e.Chart},
		{"reason", e.Reason},
		{"tracking_issue", e.TrackingIssue},
		{"exit_criteria", e.ExitCriteria},
		{"added", e.Added},
		{"added_by", e.AddedBy},
	}
	for _, f := range required {
		if strings.TrimSpace(f.val) == "" {
			return fmt.Errorf("skip entry index %d: %w: %q", idx, errSkipsMissingRequired, f.name)
		}
	}

	if err := validateChartName(e.Chart); err != nil {
		return fmt.Errorf("skip entry index %d: chart %q: %w", idx, e.Chart, err)
	}

	if err := validateTrackingIssue(e.TrackingIssue); err != nil {
		return fmt.Errorf("skip entry index %d (chart=%s): tracking_issue %q: %w",
			idx, e.Chart, e.TrackingIssue, err)
	}

	return nil
}

// validateChartName rejects shapes that would be unsafe to interpolate into
// log lines, shell-out summaries, or any future path fragment.
func validateChartName(name string) error {
	if strings.ContainsAny(name, " \t\n\r") {
		return errSkipsChartHasWhitespace
	}
	if strings.Contains(name, "/") {
		return errSkipsChartHasSlash
	}
	if strings.Contains(name, `\`) {
		return errSkipsChartHasBackslash
	}
	if strings.Contains(name, "..") {
		return errSkipsChartHasPathTraversal
	}
	return nil
}

// validateTrackingIssue allows the literal "needs new issue" sentinel OR a
// URL containing "github.com". The audit script (future subtask) is the
// canonical place for stricter URL validation.
func validateTrackingIssue(v string) error {
	v = strings.TrimSpace(v)
	if v == skipsNeedsNewIssueSentinel {
		return nil
	}
	if strings.Contains(v, "github.com") &&
		(strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://")) {
		return nil
	}
	return errSkipsBadTrackingIssue
}

// IsSkipped reports whether the given chart name is in the skip list and, if
// so, returns a pointer to its entry. The returned pointer is into the
// config's internal slice — callers must not mutate it.
func (s *SkipsConfig) IsSkipped(chart string) (bool, *SkipEntry) {
	if s == nil || s.byChart == nil {
		return false, nil
	}
	if e, ok := s.byChart[chart]; ok {
		return true, e
	}
	return false, nil
}
