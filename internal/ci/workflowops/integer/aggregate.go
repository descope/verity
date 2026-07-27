package integer

import (
	"context"
	"fmt"
	"io"
	"strings"
)

const shardSize = 250

func (aggregator Aggregator) Aggregate(ctx context.Context, options *Options) (Result, error) {
	if err := validateOptions(options); err != nil {
		return Result{}, err
	}
	input, err := parseInputs(ctx, options)
	if err != nil {
		return Result{}, err
	}
	failures := aggregateFailures(input, options)
	result := Result{
		ExpectedCount: len(input.plan),
		ShardCount:    (len(input.plan) + shardSize - 1) / shardSize,
		Failures:      failures,
	}
	if len(input.plan) == 0 && len(failures) == 0 {
		if options.ChildResult.String() != "skipped" {
			return result, &FailureSetError{Count: 1}
		}
		result.Message = "No Integer child builds were dispatched."
		return result, aggregator.writeSuccess(result)
	}
	if err := aggregator.writeStderr("Integer child shard matrix concluded: " + options.ChildResult.String() + "\n"); err != nil {
		return Result{}, err
	}
	if len(failures) > 0 {
		if err := aggregator.writeFailures(failures, options.BatchID); err != nil {
			return Result{}, err
		}
		return result, &FailureSetError{Count: len(failures)}
	}
	if options.ChildResult.String() != "success" {
		return result, &FailureSetError{Count: 1}
	}
	result.Message = fmt.Sprintf("All %d planned Integer child builds succeeded across %d shard(s).", result.ExpectedCount, result.ShardCount)
	return result, aggregator.writeSuccess(result)
}

func validateOptions(options *Options) error {
	if options == nil || options.ExpectedPath == "" || options.ResultsDir == "" || options.Repository.String() == "" || options.RunID < 1 || !batchPattern.MatchString(options.BatchID) || !sourcePattern.MatchString(options.SourceSHA) {
		return fmt.Errorf("%w: paths, repository, run ID, and batch ID are required", ErrInvalidAggregation)
	}
	if options.ChildResult.String() == "" {
		return fmt.Errorf("%w: child result is required", ErrInvalidAggregation)
	}
	runID, _, err := parseBatchIdentity(options.BatchID)
	if err != nil || runID != options.RunID {
		return fmt.Errorf("%w: batch does not identify run %d", ErrInvalidAggregation, options.RunID)
	}
	return nil
}

func (aggregator Aggregator) writeFailures(failures []Failure, batchID string) error {
	var summary strings.Builder
	summary.WriteString("## Failed Integer matrix entries")
	for _, failure := range failures {
		line := fmt.Sprintf("- %s:%s-%s — shard=%d; stage=%s; batch=%s; run=%s; artifact=%s", failure.Image, failure.Version, failure.Type, failure.Shard, failure.Stage, batchID, failure.RunURL, failure.Artifact)
		if err := write(aggregator.Stderr, line+"\n", "aggregation failure"); err != nil {
			return err
		}
		summary.WriteByte('\n')
		summary.WriteString(line)
	}
	summary.WriteByte('\n')
	return write(aggregator.Summary, summary.String(), "aggregation summary")
}

func (aggregator Aggregator) writeSuccess(result Result) error {
	return write(aggregator.Stdout, result.Message+"\n", "aggregation output")
}

func (aggregator Aggregator) writeStderr(message string) error {
	return write(aggregator.Stderr, message, "aggregation diagnostic")
}

func write(writer io.Writer, message, label string) error {
	if writer == nil {
		return nil
	}
	if _, err := io.WriteString(writer, message); err != nil {
		return fmt.Errorf("write %s: %w", label, err)
	}
	return nil
}
