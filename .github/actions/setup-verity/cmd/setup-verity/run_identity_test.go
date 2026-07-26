package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/buildmetadata"
)

func TestVerifyCurrentRunAttempt_accepts_exact_repository_workflow_and_attempt(t *testing.T) {
	// Given the API returns the exact current attempt and reusable producer identity.
	response := exactRunAttemptResponse()
	server := runAttemptServer(t, &response, nil)
	options := exactRemoteOptions(server.URL)

	// When current-run identity is verified.
	err := verifyCurrentRunAttempt(context.Background(), options)

	// Then run ID, attempt, source, repository, and reusable workflow all pass.
	require.NoError(t, err)
}

func TestVerifyCurrentRunAttempt_accepts_protected_producer_identity(t *testing.T) {
	// Given the protected build producer is the exact referenced workflow.
	response := exactRunAttemptResponse()
	response.ReferencedWorkflows[0].Path = "verity-org/verity/.github/workflows/build-verity-protected.yaml@" + testActionSourceSHA
	server := runAttemptServer(t, &response, nil)
	options := exactRemoteOptions(server.URL)
	options.ProtectedAttestation = true

	// When current-run identity is verified in protected mode.
	err := verifyCurrentRunAttempt(context.Background(), options)

	// Then the protected producer identity is accepted.
	require.NoError(t, err)
}

func TestVerifyCurrentRunAttempt_accepts_pull_request_merge_workflow_identity(t *testing.T) {
	// Given the API reports the current source SHA and a distinct synthetic PR merge workflow SHA.
	response := exactRunAttemptResponse()
	mergeSHA := strings.Repeat("c", 40)
	response.ReferencedWorkflows[0] = remoteReferencedWorkflow{
		Path: "verity-org/verity/.github/workflows/build-verity.yaml@" + mergeSHA,
		SHA:  mergeSHA,
		Ref:  "refs/pull/1024/merge",
	}
	server := runAttemptServer(t, &response, nil)

	// When current-run identity is verified for an unprotected PR build.
	err := verifyCurrentRunAttempt(context.Background(), exactRemoteOptions(server.URL))

	// Then the exact referenced PR merge workflow is accepted without conflating it with head_sha.
	require.NoError(t, err)
}

func TestVerifyCurrentRunAttempt_rejects_hostile_identity_mutations(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*remoteRunAttempt)
	}{
		{name: "wrong run ID", mutate: func(run *remoteRunAttempt) { run.ID++ }},
		{name: "wrong run attempt", mutate: func(run *remoteRunAttempt) { run.RunAttempt++ }},
		{name: "wrong source SHA", mutate: func(run *remoteRunAttempt) { run.HeadSHA = strings.Repeat("b", 40) }},
		{name: "wrong repository", mutate: func(run *remoteRunAttempt) { run.Repository.FullName = "attacker/verity" }},
		{name: "missing repository ID", mutate: func(run *remoteRunAttempt) { run.Repository.ID = 0 }},
		{name: "wrong workflow", mutate: func(run *remoteRunAttempt) {
			run.ReferencedWorkflows[0].Path = "verity-org/verity/.github/workflows/other.yaml@refs/heads/main"
		}},
		{name: "wrong workflow SHA", mutate: func(run *remoteRunAttempt) {
			run.ReferencedWorkflows[0].SHA = strings.Repeat("b", 40)
		}},
		{name: "duplicate workflow identity", mutate: func(run *remoteRunAttempt) {
			run.ReferencedWorkflows = append(run.ReferencedWorkflows, run.ReferencedWorkflows[0])
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given one mismatched run-attempt response.
			response := exactRunAttemptResponse()
			test.mutate(&response)
			server := runAttemptServer(t, &response, nil)

			// When the setup boundary verifies current-run identity.
			err := verifyCurrentRunAttempt(context.Background(), exactRemoteOptions(server.URL))

			// Then stale or cross-repository workflow state is rejected.
			require.Error(t, err)
			assert.ErrorIs(t, err, buildmetadata.ErrArtifactMismatch)
		})
	}
}

func TestVerifyCurrentRunAttempt_requests_exact_attempt_endpoint(t *testing.T) {
	// Given an observable workflow-run API.
	var requested string
	response := exactRunAttemptResponse()
	server := runAttemptServer(t, &response, &requested)

	// When current-run identity is verified.
	err := verifyCurrentRunAttempt(context.Background(), exactRemoteOptions(server.URL))

	// Then the request is scoped to the exact run attempt, not mutable latest state.
	require.NoError(t, err)
	assert.Equal(t, "/repos/verity-org/verity/actions/runs/42/attempts/2", requested)
}

func exactRemoteOptions(apiBaseURL string) *remoteOptions {
	name := "verity-linux-amd64-" + testActionBuildKey + "-42-2"
	return &remoteOptions{
		APIBaseURL: apiBaseURL, Token: "token", Repository: "verity-org/verity", RunID: 42, RunAttempt: 2,
		ArtifactName: name, ArtifactDigest: "sha256:" + strings.Repeat("b", 64),
		Identity: artifactIdentity{SourceSHA: testActionSourceSHA, BuildKey: testActionBuildKey},
	}
}

func exactRunAttemptResponse() remoteRunAttempt {
	return remoteRunAttempt{
		ID: 42, RunAttempt: 2, HeadSHA: testActionSourceSHA,
		Repository: remoteRepository{ID: 99, FullName: "verity-org/verity"},
		ReferencedWorkflows: []remoteReferencedWorkflow{{
			Path: "verity-org/verity/.github/workflows/build-verity.yaml@" + testActionSourceSHA,
			SHA:  testActionSourceSHA,
			Ref:  "refs/heads/main",
		}},
	}
}

func runAttemptServer(t *testing.T, response *remoteRunAttempt, requested *string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if requested != nil {
			*requested = request.URL.Path
		}
		assert.Equal(t, "Bearer token", request.Header.Get("Authorization"))
		assert.Equal(t, githubAPIVersion, request.Header.Get("X-GitHub-Api-Version"))
		writer.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(writer).Encode(response))
	}))
}
