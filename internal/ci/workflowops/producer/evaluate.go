package producer

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/verity-org/verity/internal/ci/workflowops/githubapi"
)

var activeStatuses = []string{"queued", "in_progress", "requested", "waiting"}

const defaultAPITimeout = 30 * time.Second

func (current *cycle) active(ctx context.Context) ([]string, error) {
	unique := make(map[string]string)
	for _, workflow := range current.options.Workflows {
		for _, status := range activeStatuses {
			runs, err := current.listRuns(ctx, githubapi.ListRunsRequest{
				Repository: current.options.Repository, Workflow: workflow,
				Branch: current.options.Branch, Status: status,
			})
			if err != nil {
				return nil, fmt.Errorf("list active %s runs: %w", workflow, err)
			}
			filtered := current.filter(runs, workflow, status)
			for index := range filtered {
				run := filtered[index]
				key := fmt.Sprintf("%s:%d", workflow, run.ID)
				unique[key] = fmt.Sprintf("%s\t%d\t%s\t%s\t%s", workflow, run.ID, run.Status, run.CreatedAt.UTC().Format(time.RFC3339), run.URL)
			}
		}
	}
	active := make([]string, 0, len(unique))
	for _, line := range unique {
		active = append(active, line)
	}
	sort.Strings(active)
	return active, nil
}

func (current *cycle) completed(ctx context.Context) (bool, Result, error) {
	if current.options.Batch != nil {
		return current.completedBatch(ctx)
	}
	outputs := make([]Output, 0, len(current.options.Workflows))
	for _, workflow := range current.options.Workflows {
		runs, err := current.completedRuns(ctx, workflow)
		if err != nil {
			return false, Result{}, err
		}
		expected, exists := current.expectedRun(workflow)
		if !exists {
			return false, Result{}, fmt.Errorf("%w: exact identity missing for %s", ErrInvalidOptions, workflow)
		}
		if len(runs) == 0 {
			if err := current.waiter.write(fmt.Sprintf("Waiting for exact %s producer run %d-%d.\n", workflow, expected.ID(), expected.Attempt())); err != nil {
				return false, Result{}, err
			}
			return false, Result{}, nil
		}
		run := runs[0]
		if run.Conclusion != "success" {
			return false, Result{}, &FailureError{Workflow: "Exact " + workflow + " producer", Conclusion: run.Conclusion, URL: run.URL}
		}
		name := strings.ReplaceAll(strings.TrimSuffix(workflow, ".yaml"), "-", "_") + "_batch_id"
		outputs = append(outputs, Output{Name: name, Value: fmt.Sprintf("%d-%d", run.ID, run.Attempt)})
		if err := current.waiter.write(fmt.Sprintf("Exact %s producer succeeded at %s: %d-%d\n", workflow, run.CreatedAt.UTC().Format(time.RFC3339), run.ID, run.Attempt)); err != nil {
			return false, Result{}, err
		}
	}
	return true, Result{Outputs: outputs}, nil
}

func (current *cycle) completedBatch(ctx context.Context) (bool, Result, error) {
	for _, workflow := range current.options.Workflows {
		runs, err := current.completedRuns(ctx, workflow)
		if err != nil {
			return false, Result{}, err
		}
		if len(runs) < current.options.ExpectedRuns {
			if err := current.waiter.write(fmt.Sprintf("Waiting for %s batch %s: %d/%d completed.\n", workflow, current.options.Batch, len(runs), current.options.ExpectedRuns)); err != nil {
				return false, Result{}, err
			}
			return false, Result{}, nil
		}
		if len(runs) > current.options.ExpectedRuns {
			return false, Result{}, &CountError{Workflow: workflow, Expected: current.options.ExpectedRuns, Actual: len(runs)}
		}
		for index := range runs {
			run := runs[index]
			if run.Conclusion != "success" {
				return false, Result{}, &FailureError{Workflow: workflow + " batch " + current.options.Batch.String(), Conclusion: run.Conclusion, URL: run.URL}
			}
		}
		if err := current.waiter.write(fmt.Sprintf("All %d %s batch %s runs succeeded.\n", current.options.ExpectedRuns, workflow, current.options.Batch)); err != nil {
			return false, Result{}, err
		}
	}
	return true, Result{}, nil
}

func (current *cycle) completedRuns(ctx context.Context, workflow string) ([]githubapi.WorkflowRun, error) {
	runs, err := current.listRuns(ctx, githubapi.ListRunsRequest{
		Repository: current.options.Repository, Workflow: workflow,
		Branch: current.options.Branch, Status: "completed",
	})
	if err != nil {
		return nil, fmt.Errorf("list completed %s runs: %w", workflow, err)
	}
	runs = current.filter(runs, workflow, "completed")
	seen := make(map[int64]struct{}, len(runs))
	for index := range runs {
		run := runs[index]
		if _, exists := seen[run.ID]; exists {
			return nil, fmt.Errorf("duplicate completed %s run ID %d: %w", workflow, run.ID, ErrProducerFailed)
		}
		seen[run.ID] = struct{}{}
	}
	sort.Slice(runs, func(left, right int) bool { return runs[left].CreatedAt.Before(runs[right].CreatedAt) })
	return runs, nil
}

func (current *cycle) filter(runs []githubapi.WorkflowRun, workflow, status string) []githubapi.WorkflowRun {
	filtered := make([]githubapi.WorkflowRun, 0, len(runs))
	for index := range runs {
		run := runs[index]
		if run.Status != status || run.HeadBranch != current.options.Branch || run.CreatedAt.Before(current.cutoff) {
			continue
		}
		if run.HeadRepository != current.options.Repository.String() || run.HeadSHA != current.options.SourceSHA {
			continue
		}
		if current.options.Event != "" && run.Event != current.options.Event {
			continue
		}
		if current.options.Batch != nil && !strings.HasSuffix(run.DisplayTitle, " [batch "+current.options.Batch.String()+"]") {
			continue
		}
		if current.options.Batch == nil {
			expected, exists := current.expectedRun(workflow)
			if !exists || run.ID != expected.ID() || run.Attempt != expected.Attempt() {
				continue
			}
		}
		filtered = append(filtered, run)
	}
	return filtered
}

func (current *cycle) expectedRun(workflow string) (ExpectedRun, bool) {
	for index := range current.options.ExactRuns {
		run := current.options.ExactRuns[index]
		if run.Workflow() == workflow {
			return run, true
		}
	}
	return ExpectedRun{}, false
}

func (current *cycle) listRuns(ctx context.Context, request githubapi.ListRunsRequest) ([]githubapi.WorkflowRun, error) {
	timeout := current.options.APITimeout
	if timeout <= 0 {
		timeout = defaultAPITimeout
	}
	attemptCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return current.waiter.GitHub.ListWorkflowRuns(attemptCtx, request)
}
