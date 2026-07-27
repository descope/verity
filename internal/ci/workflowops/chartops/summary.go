package chartops

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
)

var (
	ErrInvalidSummary      = errors.New("invalid chart summary input")
	ErrInvalidSkipSentinel = errors.New("invalid chart skip sentinel")
)

var chartNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

type SummaryProfile string

const (
	ProfileStandard   SummaryProfile = "standard"
	ProfilePrivileged SummaryProfile = "privileged"
)

type SummaryInput struct {
	Chart    string
	Outcome  string
	Profile  SummaryProfile
	SkipFile string
}

func BuildSummary(input SummaryInput) (string, error) {
	if !chartNamePattern.MatchString(input.Chart) {
		return "", fmt.Errorf("%w: invalid chart %q", ErrInvalidSummary, input.Chart)
	}
	switch input.Outcome {
	case "success", "failure", "cancelled", "skipped":
	default:
		return "", fmt.Errorf("%w: invalid outcome %q", ErrInvalidSummary, input.Outcome)
	}

	switch input.Profile {
	case "", ProfileStandard:
		return standardSummary(input)
	case ProfilePrivileged:
		return privilegedSummary(input), nil
	default:
		return "", fmt.Errorf("%w: invalid profile %q", ErrInvalidSummary, input.Profile)
	}
}

func standardSummary(input SummaryInput) (string, error) {
	if input.Outcome == "success" {
		skip, found, err := readSkipSentinel(input)
		if err != nil {
			return "", err
		}
		if found {
			return fmt.Sprintf(
				"## ⚠️ %s: skipped (SKIPS.yaml)\n\n- **reason:** `%s`\n- **tracking_issue:** %s\n- **exit_criteria:** %s\n\nSkip is explicit and tracked. Remove from `test/chart-integration/SKIPS.yaml` once exit criteria are met.\n",
				input.Chart, skip.reason, skip.trackingIssue, skip.exitCriteria,
			), nil
		}
		return fmt.Sprintf("## ✅ %s: success\n", input.Chart), nil
	}
	if input.Outcome == "failure" {
		return fmt.Sprintf(
			"## ❌ %s: failure\n\nSmoke test reported a failure. Inspect the run log above and the uploaded `diagnostics-%s` artifact for kubectl cluster-info dump and kind logs.\n",
			input.Chart, input.Chart,
		), nil
	}
	return fmt.Sprintf("## %s: %s\n", input.Chart, input.Outcome), nil
}

func privilegedSummary(input SummaryInput) string {
	if input.Outcome == "success" {
		return fmt.Sprintf("## ✅ %s privileged: success\n", input.Chart)
	}
	return fmt.Sprintf(
		"## ❌ %s privileged: %s\n\nInspect diagnostics-%s-privileged for cluster-info and kind logs.\n",
		input.Chart, input.Outcome, input.Chart,
	)
}

type skipSentinel struct {
	reason        string
	trackingIssue string
	exitCriteria  string
}

func readSkipSentinel(input SummaryInput) (skipSentinel, bool, error) {
	path := input.SkipFile
	if path == "" {
		path = "_skip-" + input.Chart + ".txt"
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return skipSentinel{}, false, nil
	}
	if err != nil {
		return skipSentinel{}, false, fmt.Errorf("read chart skip sentinel %q: %w", path, err)
	}

	fields := make(map[string]string)
	for line := range strings.SplitSeq(string(data), "\n") {
		name, value, found := strings.Cut(line, "=")
		if found {
			fields[strings.TrimSpace(name)] = strings.TrimSpace(value)
		}
	}
	if fields["chart"] != input.Chart || fields["reason"] == "" || fields["tracking_issue"] == "" || fields["exit_criteria"] == "" {
		return skipSentinel{}, false, fmt.Errorf("%w: %s", ErrInvalidSkipSentinel, path)
	}
	return skipSentinel{
		reason: fields["reason"], trackingIssue: fields["tracking_issue"], exitCriteria: fields["exit_criteria"],
	}, true, nil
}
