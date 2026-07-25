package integer

import (
	"fmt"
	"strconv"
	"strings"
)

func aggregateFailures(input parsedInput, options *Options) []Failure {
	byIdentity := make(map[string][]childReport)
	for index := range input.reports {
		report := input.reports[index]
		key := identity(report.Image, report.Version, report.Type)
		byIdentity[key] = append(byIdentity[key], report)
	}

	failures := make([]Failure, 0)
	expected := make(map[string]struct{}, len(input.plan))
	for index, entry := range input.plan {
		key := identity(entry.Name, entry.Version, entry.Type)
		expected[key] = struct{}{}
		set := expectedReports{entry: entry, shard: index/shardSize + 1, reports: byIdentity[key]}
		failures = append(failures, set.failures(options)...)
	}
	for index := range input.reports {
		report := input.reports[index]
		if _, exists := expected[identity(report.Image, report.Version, report.Type)]; exists {
			continue
		}
		entry := planEntry{Name: report.Image, Version: report.Version, Type: report.Type}
		spec := failureSpec{entry: entry, shard: report.Shard, stage: "unexpected-child-report"}
		failures = append(failures, newFailure(spec, options))
	}
	return failures
}

type expectedReports struct {
	entry   planEntry
	shard   int
	reports []childReport
}

func (set *expectedReports) failures(options *Options) []Failure {
	if len(set.reports) == 0 {
		spec := failureSpec{entry: set.entry, shard: set.shard, stage: "matrix-dispatch-or-report"}
		return []Failure{newFailure(spec, options)}
	}

	failures := make([]Failure, 0)
	accepted := false
	for index := range set.reports {
		report := set.reports[index]
		stage := reportFailureStage(&report, set.shard, options)
		if stage != "" {
			spec := failureSpec{entry: set.entry, shard: set.shard, stage: stage}
			failures = append(failures, newFailure(spec, options))
			continue
		}
		if accepted {
			spec := failureSpec{entry: set.entry, shard: set.shard, stage: "duplicate-child-report"}
			failures = append(failures, newFailure(spec, options))
			continue
		}
		accepted = true
	}
	return failures
}

func reportFailureStage(report *childReport, expectedShard int, options *Options) string {
	if stage := correlationMismatch(report, options); stage != "" {
		return stage
	}
	if report.Shard != expectedShard {
		return "wrong-shard-report"
	}
	if report.Status == "success" {
		return ""
	}
	if report.FailureStage == "" {
		return "unknown"
	}
	return report.FailureStage
}

func correlationMismatch(report *childReport, options *Options) string {
	_, attempt, err := parseBatchIdentity(options.BatchID)
	if err != nil || report.BatchID != options.BatchID {
		return "batch-mismatch"
	}
	if report.RunID != strconv.FormatInt(options.RunID, 10) {
		return "run-mismatch"
	}
	if report.RunAttempt != attempt {
		return "run-attempt-mismatch"
	}
	if report.SourceSHA != options.SourceSHA {
		return "source-mismatch"
	}
	if report.Repository != options.Repository.String() {
		return "repository-mismatch"
	}
	return ""
}

func parseBatchIdentity(value string) (runID, attempt int64, err error) {
	parts := strings.Split(value, "-")
	if len(parts) != 2 {
		return 0, 0, ErrInvalidAggregation
	}
	runID, err = strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("parse batch run ID: %w", err)
	}
	attempt, err = strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("parse batch run attempt: %w", err)
	}
	return runID, attempt, nil
}

type failureSpec struct {
	entry planEntry
	shard int
	stage string
}

func newFailure(spec failureSpec, options *Options) Failure {
	runID := strconv.FormatInt(options.RunID, 10)
	return Failure{
		Image: spec.entry.Name, Version: spec.entry.Version, Type: spec.entry.Type,
		Shard: spec.shard, Stage: spec.stage, RunID: runID,
		RunURL:   fmt.Sprintf("https://github.com/%s/actions/runs/%s", options.Repository.String(), runID),
		Artifact: "integer-build-result-" + strings.ReplaceAll(spec.entry.Name, "/", "-") + "-" + spec.entry.Version + "-" + spec.entry.Type,
	}
}
