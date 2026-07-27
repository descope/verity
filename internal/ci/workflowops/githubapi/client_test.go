package githubapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestClientListWorkflowRuns_rejects_fork_repository_metadata(t *testing.T) {
	// Given: a success-looking run whose trusted GitHub metadata identifies a fork.
	repository, err := NewRepository("verity-org/verity")
	require.NoError(t, err)
	client := Client{Runner: staticRunner{response: Response{
		StatusCode: http.StatusOK,
		Body: []byte(`{"total_count":1,"workflow_runs":[{
  "id":999,"run_attempt":7,"status":"completed","conclusion":"success",
  "created_at":"2026-07-24T09:00:00Z","html_url":"https://example.invalid/999",
  "event":"workflow_dispatch","display_title":"spoof [batch 42-1]","head_branch":"main",
  "head_sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  "head_repository":{"full_name":"attacker/fork"}
}]}`),
	}}}

	// When: the run crosses the GitHub API boundary.
	_, err = client.ListWorkflowRuns(t.Context(), ListRunsRequest{
		Repository: repository, Workflow: "patch-image.yaml", Branch: "main", Status: "completed",
	})

	// Then: repository spoofing is rejected before producer evaluation.
	require.ErrorIs(t, err, ErrInvalidResponse)
}

func TestHTTPRunner_honors_request_context_deadline(t *testing.T) {
	// Given: a GitHub-compatible endpoint that does not answer before cancellation.
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		<-request.Context().Done()
	}))
	defer server.Close()
	runner, err := NewHTTPRunner(server.URL, "token")
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	t.Cleanup(cancel)
	started := time.Now()

	// When: the outbound request exceeds the caller deadline.
	_, err = runner.Do(ctx, Request{Method: http.MethodGet, Path: "/slow"})

	// Then: the API call returns near the caller deadline.
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Less(t, time.Since(started), 150*time.Millisecond)
}

func TestClientListWorkflowRuns_rejects_malformed_external_json(t *testing.T) {
	// Given: GitHub returns HTTP 200 with a misleading, wrongly typed run identifier.
	repository, err := NewRepository("verity-org/verity")
	require.NoError(t, err)
	client := Client{Runner: staticRunner{response: Response{
		StatusCode: 200,
		Body:       []byte(`{"total_count":1,"workflow_runs":[{"id":"success"}]}`),
	}}}

	// When: workflow runs cross the typed API boundary.
	_, err = client.ListWorkflowRuns(t.Context(), ListRunsRequest{
		Repository: repository,
		Workflow:   "integer-orchestrator.yaml",
		Branch:     "main",
		Status:     "completed",
	})

	// Then: malformed external JSON fails closed.
	require.ErrorIs(t, err, ErrInvalidResponse)
}

func TestHTTPRunner_authorizes_without_exposing_token_in_errors(t *testing.T) {
	// Given: a GitHub-compatible endpoint that verifies auth and returns a retryable failure.
	token := "sensitive-token"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		require.Equal(t, "Bearer "+token, request.Header.Get("Authorization"))
		writer.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	runner, err := NewHTTPRunner(server.URL, token)
	require.NoError(t, err)
	repository, err := NewRepository("verity-org/verity")
	require.NoError(t, err)

	// When: a content mutation receives an upstream failure.
	err = (Client{Runner: runner}).PutContent(t.Context(), &PutContentRequest{
		Repository: repository,
		RemotePath: "reports/example/latest.json",
		Branch:     "reports",
		Message:    "update report",
		Content:    "e30=",
	})

	// Then: the typed status is retryable and no credential enters the error text.
	require.Error(t, err)
	var statusErr *StatusError
	require.ErrorAs(t, err, &statusErr)
	require.True(t, statusErr.Retryable())
	require.NotContains(t, err.Error(), token)
}

type staticRunner struct {
	response Response
	err      error
}

func (runner staticRunner) Do(context.Context, Request) (Response, error) {
	return runner.response, runner.err
}
