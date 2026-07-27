package producer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/verity-org/verity/internal/ci/workflowops/githubapi"
)

var (
	ErrInvalidOptions = errors.New("invalid producer wait options")
	ErrProducerFailed = errors.New("producer workflow failed")
	ErrWaitTimeout    = errors.New("producer wait timed out")
)

var (
	eventPattern    = regexp.MustCompile(`^[a-z_]+$`)
	batchPattern    = regexp.MustCompile(`^([1-9]\d*)-([1-9]\d*)$`)
	sourcePattern   = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)
	workflowPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
)

type BatchID struct {
	value      string
	runID      int64
	runAttempt int64
}

func ParseBatchID(value string) (BatchID, error) {
	matches := batchPattern.FindStringSubmatch(value)
	if matches == nil {
		return BatchID{}, fmt.Errorf("%w: batch must have form RUN_ID-RUN_ATTEMPT", ErrInvalidOptions)
	}
	runID, err := strconv.ParseInt(matches[1], 10, 64)
	if err != nil {
		return BatchID{}, fmt.Errorf("%w: parse batch run ID", ErrInvalidOptions)
	}
	attempt, err := strconv.ParseInt(matches[2], 10, 64)
	if err != nil {
		return BatchID{}, fmt.Errorf("%w: parse batch run attempt", ErrInvalidOptions)
	}
	return BatchID{value: value, runID: runID, runAttempt: attempt}, nil
}

func (batch BatchID) String() string {
	return batch.value
}

func (batch BatchID) RunID() int64 {
	return batch.runID
}

func (batch BatchID) Attempt() int64 {
	return batch.runAttempt
}

type ExpectedRun struct {
	workflow string
	id       int64
	attempt  int64
}

func NewExpectedRun(workflow string, id, attempt int64) (ExpectedRun, error) {
	if !workflowPattern.MatchString(workflow) || id < 1 || attempt < 1 {
		return ExpectedRun{}, fmt.Errorf("%w: exact run identity is invalid", ErrInvalidOptions)
	}
	return ExpectedRun{workflow: workflow, id: id, attempt: attempt}, nil
}

func (run ExpectedRun) Workflow() string {
	return run.workflow
}

func (run ExpectedRun) ID() int64 {
	return run.id
}

func (run ExpectedRun) Attempt() int64 {
	return run.attempt
}

type Clock interface {
	Now() time.Time
}

type Sleeper interface {
	Wait(context.Context, time.Duration) error
}

type GitHub interface {
	ListWorkflowRuns(context.Context, githubapi.ListRunsRequest) ([]githubapi.WorkflowRun, error)
}

type Options struct {
	Repository   githubapi.Repository
	Workflows    []string
	Branch       string
	Lookback     time.Duration
	Timeout      time.Duration
	Interval     time.Duration
	Event        string
	Batch        *BatchID
	ExpectedRuns int
	ExactRuns    []ExpectedRun
	SourceSHA    string
	APITimeout   time.Duration
}

type Output struct {
	Name  string
	Value string
}

type Result struct {
	Outputs []Output
}

type Waiter struct {
	GitHub  GitHub
	Clock   Clock
	Sleeper Sleeper
	Stdout  io.Writer
}

type FailureError struct {
	Workflow   string
	Conclusion string
	URL        string
}

func (err *FailureError) Error() string {
	return fmt.Sprintf("%s run did not succeed: %s (%s)", err.Workflow, err.Conclusion, err.URL)
}

func (err *FailureError) Is(target error) bool {
	return target == ErrProducerFailed
}

type TimeoutError struct {
	Active []string
}

func (err *TimeoutError) Error() string {
	if len(err.Active) == 0 {
		return ErrWaitTimeout.Error()
	}
	return ErrWaitTimeout.Error() + ": " + strings.Join(err.Active, "; ")
}

func (err *TimeoutError) Is(target error) bool {
	return target == ErrWaitTimeout
}

type CountError struct {
	Workflow string
	Expected int
	Actual   int
}

func (err *CountError) Error() string {
	return fmt.Sprintf("%s exact batch expected %d run(s), found %d", err.Workflow, err.Expected, err.Actual)
}

func (err *CountError) Is(target error) bool {
	return target == ErrProducerFailed
}
