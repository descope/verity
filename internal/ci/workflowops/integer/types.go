package integer

import (
	"errors"
	"fmt"
	"io"
	"regexp"

	"github.com/verity-org/verity/internal/ci/workflowops/githubapi"
)

var (
	ErrInvalidAggregation = errors.New("invalid Integer aggregation request")
	ErrInvalidPlan        = errors.New("invalid or duplicate Integer build plan")
	ErrInvalidChildReport = errors.New("invalid Integer child report")
	ErrAggregationFailed  = errors.New("integer aggregation failed")
)

var (
	batchPattern  = regexp.MustCompile(`^[1-9]\d*-[1-9]\d*$`)
	sourcePattern = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)
)

type ChildResult struct {
	value string
}

func ParseChildResult(value string) (ChildResult, error) {
	switch value {
	case "success", "failure", "cancelled", "skipped":
		return ChildResult{value: value}, nil
	default:
		return ChildResult{}, fmt.Errorf("%w: unsupported child result %q", ErrInvalidAggregation, value)
	}
}

func (result ChildResult) String() string {
	return result.value
}

type Options struct {
	ExpectedPath string
	ResultsDir   string
	ChildResult  ChildResult
	Repository   githubapi.Repository
	RunID        int64
	BatchID      string
	SourceSHA    string
}

type Failure struct {
	Image    string
	Version  string
	Type     string
	Shard    int
	Stage    string
	RunID    string
	RunURL   string
	Artifact string
}

type Result struct {
	ExpectedCount int
	ShardCount    int
	Failures      []Failure
	Message       string
}

type Aggregator struct {
	Stdout  io.Writer
	Stderr  io.Writer
	Summary io.Writer
}

type FailureSetError struct {
	Count int
}

func (err *FailureSetError) Error() string {
	return fmt.Sprintf("%d Integer matrix entry failure(s): %v", err.Count, ErrAggregationFailed)
}

func (err *FailureSetError) Is(target error) bool {
	return target == ErrAggregationFailed
}

type planEntry struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Type    string `json:"type"`
}

type childReport struct {
	Image        string `json:"image"`
	Version      string `json:"version"`
	Type         string `json:"type"`
	Status       string `json:"status"`
	FailureStage string `json:"failure_stage"`
	RunID        string `json:"run_id"`
	RunAttempt   int64  `json:"run_attempt"`
	SourceSHA    string `json:"source_sha"`
	Repository   string `json:"repository"`
	BatchID      string `json:"batch_id"`
	Shard        int    `json:"shard"`
}

type parsedInput struct {
	plan    []planEntry
	reports []childReport
}
