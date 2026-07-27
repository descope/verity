package githubapi

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const exactSourceSHA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestClientGetWorkflowRunAttempt_returns_exact_producer_identity(t *testing.T) {
	// Given
	repository, err := NewRepository("verity-org/verity")
	require.NoError(t, err)
	client := Client{Runner: staticRunner{response: Response{
		StatusCode: http.StatusOK,
		Body: []byte(`{
  "id":42,"run_attempt":3,"status":"in_progress","conclusion":null,
  "created_at":"2026-07-17T06:00:00Z","html_url":"https://github.com/verity-org/verity/actions/runs/42",
  "event":"workflow_call","display_title":"Patch nginx","head_branch":"main",
  "head_sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  "head_repository":{"full_name":"verity-org/verity"}
}`),
	}}}

	// When
	run, err := client.GetWorkflowRunAttempt(t.Context(), GetRunAttemptRequest{
		Repository: repository, RunID: 42, RunAttempt: 3, SourceSHA: exactSourceSHA,
	})

	// Then
	require.NoError(t, err)
	assert.Equal(t, int64(42), run.ID)
	assert.Equal(t, int64(3), run.Attempt)
	assert.Equal(t, time.Date(2026, 7, 17, 6, 0, 0, 0, time.UTC), run.CreatedAt)
}

func TestClientGetWorkflowRunAttempt_rejects_wrong_source_identity(t *testing.T) {
	// Given
	repository, err := NewRepository("verity-org/verity")
	require.NoError(t, err)
	client := Client{Runner: staticRunner{response: Response{
		StatusCode: http.StatusOK,
		Body: []byte(`{
  "id":42,"run_attempt":3,"status":"in_progress","conclusion":null,
  "created_at":"2026-07-17T06:00:00Z","html_url":"https://github.com/verity-org/verity/actions/runs/42",
  "event":"workflow_call","display_title":"Patch nginx","head_branch":"main",
  "head_sha":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
  "head_repository":{"full_name":"verity-org/verity"}
}`),
	}}}

	// When
	_, err = client.GetWorkflowRunAttempt(t.Context(), GetRunAttemptRequest{
		Repository: repository, RunID: 42, RunAttempt: 3, SourceSHA: exactSourceSHA,
	})

	// Then
	require.ErrorIs(t, err, ErrInvalidResponse)
}

func TestClientGetWorkflowRunArtifact_returns_only_exact_name_and_source(t *testing.T) {
	// Given
	repository, err := NewRepository("verity-org/verity")
	require.NoError(t, err)
	client := Client{Runner: staticRunner{response: Response{
		StatusCode: http.StatusOK,
		Body: []byte(`{
  "total_count":2,
  "artifacts":[
    {"id":7,"name":"metrics-nginx-1.2.3-decoy","expired":false,"created_at":"2026-07-17T06:01:00Z","workflow_run":{"id":42,"head_sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}},
    {"id":8,"name":"metrics-nginx-1.2.3","expired":false,"created_at":"2026-07-17T06:02:00Z","workflow_run":{"id":42,"head_sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}
  ]
}`),
	}}}

	// When
	artifact, err := client.GetWorkflowRunArtifact(t.Context(), GetRunArtifactRequest{
		Repository: repository, RunID: 42, ArtifactName: "metrics-nginx-1.2.3", SourceSHA: exactSourceSHA,
	})

	// Then
	require.NoError(t, err)
	assert.Equal(t, int64(8), artifact.ID)
	assert.Equal(t, "metrics-nginx-1.2.3", artifact.Name)
}

func TestClientGetWorkflowRunArtifact_rejects_ambiguous_exact_name(t *testing.T) {
	// Given
	repository, err := NewRepository("verity-org/verity")
	require.NoError(t, err)
	client := Client{Runner: staticRunner{response: Response{
		StatusCode: http.StatusOK,
		Body: []byte(`{
  "total_count":2,
  "artifacts":[
    {"id":7,"name":"metrics-nginx-1.2.3","expired":false,"created_at":"2026-07-17T06:01:00Z","workflow_run":{"id":42,"head_sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}},
    {"id":8,"name":"metrics-nginx-1.2.3","expired":false,"created_at":"2026-07-17T06:02:00Z","workflow_run":{"id":42,"head_sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}
  ]
}`),
	}}}

	// When
	_, err = client.GetWorkflowRunArtifact(t.Context(), GetRunArtifactRequest{
		Repository: repository, RunID: 42, ArtifactName: "metrics-nginx-1.2.3", SourceSHA: exactSourceSHA,
	})

	// Then
	require.ErrorIs(t, err, ErrInvalidResponse)
}

func TestClientGetWorkflowRunArtifact_rejects_expired_or_wrong_source(t *testing.T) {
	tests := []struct {
		name      string
		expired   bool
		headSHA   string
		wantError error
	}{
		{name: "expired", expired: true, headSHA: exactSourceSHA, wantError: ErrNotFound},
		{name: "wrong source", headSHA: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", wantError: ErrInvalidResponse},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given
			repository, err := NewRepository("verity-org/verity")
			require.NoError(t, err)
			body := `{"total_count":1,"artifacts":[{"id":8,"name":"metrics-nginx-1.2.3","expired":` +
				boolJSON(test.expired) + `,"created_at":"2026-07-17T06:02:00Z","workflow_run":{"id":42,"head_sha":"` + test.headSHA + `"}}]}`
			client := Client{Runner: staticRunner{response: Response{StatusCode: http.StatusOK, Body: []byte(body)}}}

			// When
			_, err = client.GetWorkflowRunArtifact(t.Context(), GetRunArtifactRequest{
				Repository: repository, RunID: 42, ArtifactName: "metrics-nginx-1.2.3", SourceSHA: exactSourceSHA,
			})

			// Then
			require.ErrorIs(t, err, test.wantError)
		})
	}
}

func boolJSON(value bool) string {
	if value {
		return "true"
	}
	return "false"
}
