package producer

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/ci/workflowops/githubapi"
)

func TestWaiter_bounds_inflight_API_call_by_timeout(t *testing.T) {
	// Given: an API call that blocks until its context is cancelled.
	repository, err := githubapi.NewRepository("verity-org/verity")
	require.NoError(t, err)
	expected, err := NewExpectedRun("integer-orchestrator.yaml", 42, 1)
	require.NoError(t, err)
	outer, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
	t.Cleanup(cancel)
	started := time.Now()

	// When: the producer wait has a much shorter timeout than the caller.
	_, err = (Waiter{GitHub: blockingGitHub{}, Clock: wallClock{}, Sleeper: wallSleeper{}}).Wait(outer, &Options{
		Repository: repository, Workflows: []string{"integer-orchestrator.yaml"}, Branch: "main",
		Lookback: time.Hour, Timeout: 25 * time.Millisecond, Interval: time.Second,
		SourceSHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		ExactRuns: []ExpectedRun{expected},
	})

	// Then: the in-flight call is bounded by the wait timeout, not the outer deadline.
	require.ErrorIs(t, err, ErrWaitTimeout)
	assert.Less(t, time.Since(started), 200*time.Millisecond)
}

func TestWaiter_rejects_untrusted_repository_and_source_metadata(t *testing.T) {
	// Given: a success-looking batch run from a fork and unrelated source commit.
	repository, err := githubapi.NewRepository("verity-org/verity")
	require.NoError(t, err)
	now := time.Date(2026, 7, 24, 9, 0, 0, 0, time.UTC)
	batch, err := ParseBatchID("42-1")
	require.NoError(t, err)
	runs := []githubapi.WorkflowRun{{
		ID: 999, Attempt: 7, Status: "completed", Conclusion: "success", CreatedAt: now,
		URL: "https://example.invalid/999", Event: "workflow_dispatch", HeadBranch: "main",
		DisplayTitle: "spoof [batch 42-1]", HeadSHA: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		HeadRepository: "attacker/fork",
	}}

	// When: exact batch evaluation examines the hostile metadata.
	clock := &advancingClock{now: now}
	_, err = (Waiter{GitHub: staticRunsGitHub{runs: runs}, Clock: clock, Sleeper: clock}).Wait(t.Context(), &Options{
		Repository: repository, Workflows: []string{"patch-image.yaml"}, Branch: "main",
		Lookback: time.Hour, Timeout: time.Second, Interval: time.Second,
		Event: "workflow_dispatch", Batch: &batch, ExpectedRuns: 1,
		SourceSHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	})

	// Then: fork and source spoofing cannot satisfy the exact producer set.
	require.ErrorIs(t, err, ErrWaitTimeout)
}

func TestWaiter_rejects_latest_window_selection_without_exact_identity(t *testing.T) {
	// Given: a successful recent run but no exact batch or run identity selector.
	repository, err := githubapi.NewRepository("verity-org/verity")
	require.NoError(t, err)
	now := time.Date(2026, 7, 24, 9, 0, 0, 0, time.UTC)
	runs := []githubapi.WorkflowRun{{
		ID: 222, Attempt: 9, Status: "completed", Conclusion: "success", CreatedAt: now,
		URL: "https://example.invalid/222", HeadBranch: "main",
		HeadSHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", HeadRepository: repository.String(),
	}}

	// When: the waiter is asked to select by time window alone.
	_, err = (Waiter{GitHub: staticRunsGitHub{runs: runs}, Clock: fixedTimeClock{now: now}, Sleeper: wallSleeper{}}).Wait(t.Context(), &Options{
		Repository: repository, Workflows: []string{"test.yaml"}, Branch: "main",
		Lookback: time.Hour, Timeout: time.Second, Interval: time.Second,
		SourceSHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	})

	// Then: ambiguous latest-run selection fails at the boundary.
	require.ErrorIs(t, err, ErrInvalidOptions)
}

func TestWaiter_resumes_incomplete_exact_batch_until_all_runs_succeed(t *testing.T) {
	// Given: an exact batch with one completion on the first poll and two on the second.
	repository, err := githubapi.NewRepository("verity-org/verity")
	require.NoError(t, err)
	clock := &advancingClock{now: time.Date(2026, 7, 24, 9, 0, 0, 0, time.UTC)}
	github := &batchSequenceGitHub{createdAt: clock.now.Add(-time.Minute)}
	waiter := Waiter{GitHub: github, Clock: clock, Sleeper: clock}
	batch, err := ParseBatchID("12345-2")
	require.NoError(t, err)

	// When: waiting for two workflow-dispatch producers in that batch.
	result, err := waiter.Wait(t.Context(), &Options{
		Repository:   repository,
		Workflows:    []string{"patch-image.yaml"},
		Branch:       "main",
		Lookback:     time.Hour,
		Timeout:      time.Minute,
		Interval:     time.Second,
		Event:        "workflow_dispatch",
		Batch:        &batch,
		ExpectedRuns: 2,
		SourceSHA:    "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	})

	// Then: the waiter resumes after the partial poll and accepts only the exact successful batch.
	require.NoError(t, err)
	assert.Empty(t, result.Outputs)
	assert.Equal(t, 2, github.completedCalls)
	assert.Equal(t, []time.Duration{time.Second}, clock.delays)
	assert.ElementsMatch(t, []string{"queued", "in_progress", "requested", "waiting"}, github.activeStatuses[:4])
}

func TestWaiter_rejects_latest_failed_producer(t *testing.T) {
	// Given: no active run and a latest completed producer that failed.
	repository, err := githubapi.NewRepository("verity-org/verity")
	require.NoError(t, err)
	now := time.Date(2026, 7, 24, 9, 0, 0, 0, time.UTC)
	github := staticRunsGitHub{runs: []githubapi.WorkflowRun{{
		ID: 77, Attempt: 2, Status: "completed", Conclusion: "failure", CreatedAt: now.Add(-time.Minute),
		URL: "https://example.invalid/77", HeadBranch: "main",
		HeadSHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", HeadRepository: repository.String(),
	}}}
	clock := &advancingClock{now: now}
	expected, err := NewExpectedRun("integer-orchestrator.yaml", 77, 2)
	require.NoError(t, err)

	// When: the latest producer is correlated.
	_, err = (Waiter{GitHub: github, Clock: clock, Sleeper: clock}).Wait(t.Context(), &Options{
		Repository: repository, Workflows: []string{"integer-orchestrator.yaml"}, Branch: "main",
		Lookback: time.Hour, Timeout: time.Minute, Interval: time.Second,
		ExactRuns: []ExpectedRun{expected}, SourceSHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	})

	// Then: a failed conclusion cannot be reported as a successful producer batch.
	require.ErrorIs(t, err, ErrProducerFailed)
}

func TestWaiter_times_out_when_only_completed_run_is_stale(t *testing.T) {
	// Given: the only completed run predates the configured lookback window.
	repository, err := githubapi.NewRepository("verity-org/verity")
	require.NoError(t, err)
	now := time.Date(2026, 7, 24, 9, 0, 0, 0, time.UTC)
	github := staticRunsGitHub{runs: []githubapi.WorkflowRun{{
		ID: 77, Attempt: 2, Status: "completed", Conclusion: "success", CreatedAt: now.Add(-2 * time.Hour),
		URL: "https://example.invalid/77", HeadBranch: "main",
		HeadSHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", HeadRepository: repository.String(),
	}}}
	clock := &advancingClock{now: now}
	expected, err := NewExpectedRun("integer-orchestrator.yaml", 77, 2)
	require.NoError(t, err)

	// When: waiting is bounded to two polling intervals.
	_, err = (Waiter{GitHub: github, Clock: clock, Sleeper: clock}).Wait(t.Context(), &Options{
		Repository: repository, Workflows: []string{"integer-orchestrator.yaml"}, Branch: "main",
		Lookback: time.Hour, Timeout: 2 * time.Second, Interval: time.Second,
		ExactRuns: []ExpectedRun{expected}, SourceSHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	})

	// Then: stale success is ignored and the operation times out deterministically.
	require.ErrorIs(t, err, ErrWaitTimeout)
	assert.Equal(t, []time.Duration{time.Second, time.Second}, clock.delays)
}

func TestWaiter_stops_when_poll_delay_is_cancelled(t *testing.T) {
	// Given: no active or completed run and a cancellable polling delay.
	repository, err := githubapi.NewRepository("verity-org/verity")
	require.NoError(t, err)
	now := time.Date(2026, 7, 24, 9, 0, 0, 0, time.UTC)
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	sleeper := cancellingSleeper{cancel: cancel}
	expected, err := NewExpectedRun("integer-orchestrator.yaml", 77, 2)
	require.NoError(t, err)

	// When: cancellation arrives while the waiter is between polls.
	_, err = (Waiter{GitHub: staticRunsGitHub{}, Clock: fixedTimeClock{now: now}, Sleeper: sleeper}).Wait(ctx, &Options{
		Repository: repository, Workflows: []string{"integer-orchestrator.yaml"}, Branch: "main",
		Lookback: time.Hour, Timeout: time.Minute, Interval: time.Second,
		ExactRuns: []ExpectedRun{expected}, SourceSHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	})

	// Then: cancellation is returned and no resume poll is attempted.
	require.ErrorIs(t, err, context.Canceled)
}

func TestWaiter_rejects_extra_runs_in_exact_batch(t *testing.T) {
	// Given: two completed runs claim a batch that expects exactly one producer.
	repository, err := githubapi.NewRepository("verity-org/verity")
	require.NoError(t, err)
	now := time.Date(2026, 7, 24, 9, 0, 0, 0, time.UTC)
	runs := []githubapi.WorkflowRun{
		{ID: 1, Attempt: 1, Status: "completed", Conclusion: "success", CreatedAt: now, URL: "https://example.invalid/1", Event: "workflow_dispatch", DisplayTitle: "first [batch 42-1]", HeadBranch: "main", HeadSHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", HeadRepository: repository.String()},
		{ID: 2, Attempt: 1, Status: "completed", Conclusion: "success", CreatedAt: now, URL: "https://example.invalid/2", Event: "workflow_dispatch", DisplayTitle: "second [batch 42-1]", HeadBranch: "main", HeadSHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", HeadRepository: repository.String()},
	}
	batch, err := ParseBatchID("42-1")
	require.NoError(t, err)
	clock := &advancingClock{now: now}

	// When: exact-batch correlation sees more runs than declared.
	_, err = (Waiter{GitHub: staticRunsGitHub{runs: runs}, Clock: clock, Sleeper: clock}).Wait(t.Context(), &Options{
		Repository: repository, Workflows: []string{"patch-image.yaml"}, Branch: "main",
		Lookback: time.Hour, Timeout: time.Minute, Interval: time.Second,
		Event: "workflow_dispatch", Batch: &batch, ExpectedRuns: 1,
		SourceSHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	})

	// Then: the ambiguous batch fails closed.
	require.ErrorIs(t, err, ErrProducerFailed)
}

type batchSequenceGitHub struct {
	createdAt      time.Time
	completedCalls int
	activeStatuses []string
}

func (github *batchSequenceGitHub) ListWorkflowRuns(_ context.Context, request githubapi.ListRunsRequest) ([]githubapi.WorkflowRun, error) {
	if request.Status != "completed" {
		github.activeStatuses = append(github.activeStatuses, request.Status)
		return nil, nil
	}
	github.completedCalls++
	runs := []githubapi.WorkflowRun{
		{ID: 10, Attempt: 1, Status: "completed", Conclusion: "success", CreatedAt: github.createdAt, URL: "https://example.invalid/10", Event: "workflow_dispatch", DisplayTitle: "patch alpha [batch 12345-2]", HeadBranch: "main", HeadSHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", HeadRepository: "verity-org/verity"},
		{ID: 99, Attempt: 1, Status: "completed", Conclusion: "success", CreatedAt: github.createdAt, URL: "https://example.invalid/stale", Event: "workflow_dispatch", DisplayTitle: "wrong batch [batch 999-1]", HeadBranch: "main", HeadSHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", HeadRepository: "verity-org/verity"},
	}
	if github.completedCalls > 1 {
		runs = append(runs, githubapi.WorkflowRun{
			ID: 11, Attempt: 1, Status: "completed", Conclusion: "success", CreatedAt: github.createdAt,
			URL: "https://example.invalid/11", Event: "workflow_dispatch", DisplayTitle: "patch beta [batch 12345-2]", HeadBranch: "main",
			HeadSHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", HeadRepository: "verity-org/verity",
		})
	}
	return runs, nil
}

type advancingClock struct {
	now    time.Time
	delays []time.Duration
}

func (clock *advancingClock) Now() time.Time {
	return clock.now
}

func (clock *advancingClock) Wait(_ context.Context, delay time.Duration) error {
	clock.delays = append(clock.delays, delay)
	clock.now = clock.now.Add(delay)
	return nil
}

type staticRunsGitHub struct {
	runs []githubapi.WorkflowRun
}

type fixedTimeClock struct {
	now time.Time
}

func (clock fixedTimeClock) Now() time.Time {
	return clock.now
}

type cancellingSleeper struct {
	cancel context.CancelFunc
}

type blockingGitHub struct{}

func (blockingGitHub) ListWorkflowRuns(ctx context.Context, _ githubapi.ListRunsRequest) ([]githubapi.WorkflowRun, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

type wallClock struct{}

func (wallClock) Now() time.Time {
	return time.Now()
}

type wallSleeper struct{}

func (wallSleeper) Wait(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (sleeper cancellingSleeper) Wait(ctx context.Context, _ time.Duration) error {
	sleeper.cancel()
	return ctx.Err()
}

func (github staticRunsGitHub) ListWorkflowRuns(_ context.Context, request githubapi.ListRunsRequest) ([]githubapi.WorkflowRun, error) {
	if request.Status == "completed" {
		return github.runs, nil
	}
	return nil, nil
}
