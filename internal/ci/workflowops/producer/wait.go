package producer

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

type cycle struct {
	waiter  Waiter
	options *Options
	cutoff  time.Time
}

func (waiter Waiter) Wait(ctx context.Context, options *Options) (Result, error) {
	if err := validateOptions(options); err != nil {
		return Result{}, err
	}
	if waiter.GitHub == nil || waiter.Clock == nil || waiter.Sleeper == nil {
		return Result{}, fmt.Errorf("%w: GitHub client, clock, and sleeper are required", ErrInvalidOptions)
	}

	waitCtx, cancel := context.WithTimeout(ctx, options.Timeout)
	defer cancel()
	start := waiter.Clock.Now()
	current := cycle{waiter: waiter, options: options, cutoff: start.Add(-options.Lookback)}
	deadline := start.Add(options.Timeout)
	if err := waiter.write(fmt.Sprintf("Waiting for active workflow runs on %s since %s: %s\n", options.Branch, current.cutoff.UTC().Format(time.RFC3339), strings.Join(options.Workflows, " "))); err != nil {
		return Result{}, err
	}

	for {
		done, result, active, err := current.poll(waitCtx)
		if err != nil {
			return Result{}, waitContextError(ctx, waitCtx, active, err)
		}
		if done {
			return result, nil
		}

		if !waiter.Clock.Now().Before(deadline) {
			return Result{}, &TimeoutError{Active: active}
		}
		if len(active) > 0 {
			if err := waiter.write("Still waiting for producer workflows:\n" + strings.Join(active, "\n") + "\n"); err != nil {
				return Result{}, err
			}
		}
		if err := waiter.Sleeper.Wait(waitCtx, options.Interval); err != nil {
			return Result{}, waitContextError(ctx, waitCtx, active, fmt.Errorf("wait for producer poll: %w", err))
		}
	}
}

func (current *cycle) poll(ctx context.Context) (done bool, result Result, active []string, err error) {
	active, err = current.active(ctx)
	if err != nil {
		return false, Result{}, active, err
	}
	if len(active) > 0 {
		return false, Result{}, active, nil
	}
	if err := current.waiter.write("No active producer workflow runs remain.\n"); err != nil {
		return false, Result{}, active, err
	}
	done, result, err = current.completed(ctx)
	return done, result, active, err
}

func waitContextError(parent, waitCtx context.Context, active []string, operationErr error) error {
	if err := parent.Err(); err != nil {
		return err
	}
	if errors.Is(waitCtx.Err(), context.DeadlineExceeded) {
		return &TimeoutError{Active: active}
	}
	return operationErr
}

func validateOptions(options *Options) error {
	if options == nil || options.Repository.String() == "" || len(options.Workflows) == 0 || options.Branch == "" {
		return fmt.Errorf("%w: repository, workflows, and branch are required", ErrInvalidOptions)
	}
	if err := validateWorkflowNames(options.Workflows); err != nil {
		return err
	}
	if err := validateWaitTiming(options); err != nil {
		return err
	}
	return validateWaitSelectors(options)
}

func validateWorkflowNames(workflows []string) error {
	for _, workflow := range workflows {
		if !workflowPattern.MatchString(workflow) {
			return fmt.Errorf("%w: workflow name %q is unsupported", ErrInvalidOptions, workflow)
		}
	}
	return nil
}

func validateWaitTiming(options *Options) error {
	if options.Lookback <= 0 || options.Timeout <= 0 || options.Interval <= 0 || options.APITimeout < 0 {
		return fmt.Errorf("%w: lookback, timeout, and interval must be positive", ErrInvalidOptions)
	}
	return nil
}

func validateWaitSelectors(options *Options) error {
	if !sourcePattern.MatchString(options.SourceSHA) {
		return fmt.Errorf("%w: source SHA must be a lowercase 40- or 64-character digest", ErrInvalidOptions)
	}
	if options.Event != "" && !eventPattern.MatchString(options.Event) {
		return fmt.Errorf("%w: event contains unsupported characters", ErrInvalidOptions)
	}
	if options.Batch != nil && options.ExpectedRuns < 1 {
		return fmt.Errorf("%w: expected runs must be positive for a batch", ErrInvalidOptions)
	}
	return validateExactRuns(options)
}

func validateExactRuns(options *Options) error {
	if options.Batch != nil {
		if len(options.ExactRuns) > 0 {
			return fmt.Errorf("%w: batch and exact run selectors are mutually exclusive", ErrInvalidOptions)
		}
		return nil
	}
	if len(options.ExactRuns) != len(options.Workflows) {
		return fmt.Errorf("%w: every workflow requires one exact run identity", ErrInvalidOptions)
	}
	seen := make(map[string]struct{}, len(options.ExactRuns))
	for _, expected := range options.ExactRuns {
		if _, exists := seen[expected.Workflow()]; exists {
			return fmt.Errorf("%w: duplicate exact run for %s", ErrInvalidOptions, expected.Workflow())
		}
		seen[expected.Workflow()] = struct{}{}
	}
	for _, workflow := range options.Workflows {
		if _, exists := seen[workflow]; !exists {
			return fmt.Errorf("%w: exact run missing for %s", ErrInvalidOptions, workflow)
		}
	}
	return nil
}

func (waiter Waiter) write(message string) error {
	if waiter.Stdout == nil {
		return nil
	}
	if _, err := fmt.Fprint(waiter.Stdout, message); err != nil {
		return fmt.Errorf("write producer wait output: %w", err)
	}
	return nil
}
