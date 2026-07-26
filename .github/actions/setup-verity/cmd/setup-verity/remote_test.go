package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/buildmetadata"
)

func TestVerifyRemoteArtifact_accepts_exact_current_run_identity(t *testing.T) {
	// Given GitHub returns one exact current-run artifact.
	name := "verity-linux-amd64-" + testActionBuildKey + "-42-2"
	digest := "sha256:" + strings.Repeat("b", 64)
	server := fakeArtifactServer(t, name, digest, testActionSourceSHA, nil)
	outputPath := filepath.Join(t.TempDir(), "github-output")

	// When the trusted source helper verifies remote identity.
	err := verifyRemoteArtifact(context.Background(), &remoteOptions{
		APIBaseURL: server.URL, Token: "token", Repository: "verity-org/verity", RunID: 42, RunAttempt: 2,
		ArtifactName: name, ArtifactDigest: digest,
		Identity:             artifactIdentity{SourceSHA: testActionSourceSHA, BuildKey: testActionBuildKey},
		ProtectedAttestation: true,
		GitHubOutput:         outputPath,
	})

	// Then exact repository, run, source, name, digest, and build key pass.
	require.NoError(t, err)
	outputs, err := os.ReadFile(outputPath)
	require.NoError(t, err)
	assert.Equal(t, "artifact-id=7\nverify-attestation=true\n", string(outputs))
}

func TestVerifyRemoteArtifact_rejects_name_not_derived_from_build_key_before_API(t *testing.T) {
	// Given a mutable artifact name and an observable fake API.
	calls := 0
	server := fakeArtifactServer(t, "verity-latest", "sha256:"+strings.Repeat("b", 64), testActionSourceSHA, &calls)

	// When remote verification is attempted.
	err := verifyRemoteArtifact(context.Background(), &remoteOptions{
		APIBaseURL: server.URL, Token: "token", Repository: "verity-org/verity", RunID: 42, RunAttempt: 2,
		ArtifactName: "verity-latest", ArtifactDigest: "sha256:" + strings.Repeat("b", 64),
		Identity: artifactIdentity{SourceSHA: testActionSourceSHA, BuildKey: testActionBuildKey},
	})

	// Then validation fails before any network request.
	require.Error(t, err)
	assert.ErrorIs(t, err, buildmetadata.ErrArtifactMismatch)
	assert.Zero(t, calls)
}

func TestVerifyRemoteArtifact_follows_all_artifact_pages(t *testing.T) {
	// Given the exact artifact appears only on the second API page after 100 decoys.
	name := "verity-linux-amd64-" + testActionBuildKey
	digest := "sha256:" + strings.Repeat("b", 64)
	requests := make([]string, 0, 2)
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests = append(requests, request.URL.String())
		writer.Header().Set("Content-Type", "application/json")
		if request.URL.Query().Get("page") == "2" {
			writeArtifactResponse(t, writer, 101, []fakeRemoteArtifact{{ID: 707, Name: name, Digest: digest, SourceSHA: testActionSourceSHA}})
			return
		}
		decoys := make([]fakeRemoteArtifact, 100)
		for index := range decoys {
			decoys[index] = fakeRemoteArtifact{ID: int64(index + 1), Name: fmt.Sprintf("decoy-%d", index), Digest: digest, SourceSHA: testActionSourceSHA}
		}
		writer.Header().Set("Link", "<"+server.URL+request.URL.Path+"?name="+name+"&per_page=100&page=2>; rel=\"next\"")
		writeArtifactResponse(t, writer, 101, decoys)
	}))
	defer server.Close()

	// When current-run identity is verified.
	_, err := findCurrentRunArtifact(context.Background(), &remoteOptions{
		APIBaseURL: server.URL, Token: "token", Repository: "verity-org/verity", RunID: 42,
		ArtifactName: name, ArtifactDigest: digest,
		Identity: artifactIdentity{SourceSHA: testActionSourceSHA, BuildKey: testActionBuildKey},
	})

	// Then every page is searched with the exact name and maximum page size.
	require.NoError(t, err)
	require.Len(t, requests, 2)
	assert.Contains(t, requests[0], "per_page=100")
	assert.Contains(t, requests[0], "name="+name)
}

func TestArtifactEndpoint_preserves_absolute_API_path(t *testing.T) {
	// Given an API base with no path prefix.
	options := &remoteOptions{
		APIBaseURL: "https://api.example.test", Repository: "verity-org/verity", RunID: 42,
		ArtifactName: "verity-linux-amd64-" + testActionBuildKey,
	}

	// When the current-run artifact endpoint is constructed.
	endpoint, expectedPath, err := artifactEndpoint(options)

	// Then pagination compares the same absolute path sent on the wire.
	require.NoError(t, err)
	assert.Equal(t, "/repos/verity-org/verity/actions/runs/42/artifacts", endpoint.Path)
	assert.Equal(t, endpoint.Path, expectedPath)
}

func TestFindCurrentRunArtifact_rejects_truncated_pagination(t *testing.T) {
	// Given an API response declaring more artifacts than it returns without a next link.
	name := "verity-linux-amd64-" + testActionBuildKey
	digest := "sha256:" + strings.Repeat("b", 64)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, err := fmt.Fprintf(writer,
			`{"total_count":101,"artifacts":[{"id":7,"name":%q,"expired":false,"digest":%q,"workflow_run":{"id":42,"head_sha":%q}}]}`,
			name, digest, testActionSourceSHA)
		require.NoError(t, err)
	}))
	defer server.Close()

	// When the complete current-run result set is required.
	_, err := findCurrentRunArtifact(context.Background(), &remoteOptions{
		APIBaseURL: server.URL, Token: "token", Repository: "verity-org/verity", RunID: 42,
		ArtifactName: name, ArtifactDigest: digest,
		Identity: artifactIdentity{SourceSHA: testActionSourceSHA, BuildKey: testActionBuildKey},
	})

	// Then a misleading partial success response fails closed.
	require.Error(t, err)
	assert.ErrorIs(t, err, buildmetadata.ErrArtifactMismatch)
}

func TestMatchesRemoteArtifact_rejects_identity_mismatches(t *testing.T) {
	name := "verity-linux-amd64-" + testActionBuildKey
	digest := "sha256:" + strings.Repeat("b", 64)
	options := &remoteOptions{
		RunID: 42, ArtifactName: name, ArtifactDigest: digest,
		Identity: artifactIdentity{SourceSHA: testActionSourceSHA, BuildKey: testActionBuildKey},
	}
	tests := []struct {
		name   string
		mutate func(*remoteArtifact)
	}{
		{name: "wrong name", mutate: func(artifact *remoteArtifact) { artifact.Name = "verity-latest" }},
		{name: "wrong digest", mutate: func(artifact *remoteArtifact) { artifact.Digest = "sha256:" + strings.Repeat("c", 64) }},
		{name: "wrong run ID", mutate: func(artifact *remoteArtifact) { artifact.WorkflowRun.ID++ }},
		{name: "wrong source SHA", mutate: func(artifact *remoteArtifact) { artifact.WorkflowRun.HeadSHA = strings.Repeat("c", 40) }},
		{name: "expired", mutate: func(artifact *remoteArtifact) { artifact.Expired = true }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given one artifact identity mismatch.
			artifact := remoteArtifact{
				ID: 7, Name: name, Digest: digest,
				WorkflowRun: remoteWorkflowRun{ID: 42, HeadSHA: testActionSourceSHA},
			}
			test.mutate(&artifact)

			// When the API result is compared to trusted inputs.
			matched := matchesRemoteArtifact(artifact, options)

			// Then the mismatch is never accepted.
			assert.False(t, matched)
		})
	}
}

func TestVerifyRemoteArtifact_rejects_duplicate_or_later_page_conflict(t *testing.T) {
	tests := []struct {
		name       string
		secondName string
		digest     string
	}{
		{name: "duplicate exact artifact", secondName: "verity-linux-amd64-" + testActionBuildKey, digest: "sha256:" + strings.Repeat("b", 64)},
		{name: "later page digest mismatch", secondName: "verity-linux-amd64-" + testActionBuildKey, digest: "sha256:" + strings.Repeat("c", 64)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given page one contains an exact artifact and a later page conflicts with the same immutable name.
			name := "verity-linux-amd64-" + testActionBuildKey
			digest := "sha256:" + strings.Repeat("b", 64)
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				if request.URL.Query().Get("page") == "2" {
					writeArtifactResponse(t, writer, 2, []fakeRemoteArtifact{{ID: 8, Name: test.secondName, Digest: test.digest, SourceSHA: testActionSourceSHA}})
					return
				}
				writer.Header().Set("Link", "<"+server.URL+request.URL.Path+"?name="+name+"&per_page=100&page=2>; rel=\"next\"")
				writeArtifactResponse(t, writer, 2, []fakeRemoteArtifact{{ID: 7, Name: name, Digest: digest, SourceSHA: testActionSourceSHA}})
			}))
			defer server.Close()

			// When remote identity verification traverses the complete result set.
			_, err := findCurrentRunArtifact(context.Background(), &remoteOptions{
				APIBaseURL: server.URL, Token: "token", Repository: "verity-org/verity", RunID: 42,
				ArtifactName: name, ArtifactDigest: digest,
				Identity: artifactIdentity{SourceSHA: testActionSourceSHA, BuildKey: testActionBuildKey},
			})

			// Then duplicates and later-page mismatches fail closed.
			require.Error(t, err)
			assert.ErrorIs(t, err, buildmetadata.ErrArtifactMismatch)
		})
	}
}

type fakeRemoteArtifact struct {
	ID        int64
	Name      string
	Digest    string
	SourceSHA string
}

func writeArtifactResponse(t *testing.T, writer http.ResponseWriter, totalCount int, artifacts []fakeRemoteArtifact) {
	t.Helper()
	payload := struct {
		TotalCount int              `json:"total_count"`
		Artifacts  []map[string]any `json:"artifacts"`
	}{TotalCount: totalCount, Artifacts: make([]map[string]any, 0, len(artifacts))}
	for _, artifact := range artifacts {
		payload.Artifacts = append(payload.Artifacts, map[string]any{
			"id": artifact.ID, "name": artifact.Name, "expired": false, "digest": artifact.Digest,
			"workflow_run": map[string]any{"id": 42, "head_sha": artifact.SourceSHA},
		})
	}
	require.NoError(t, json.NewEncoder(writer).Encode(payload))
}

func fakeArtifactServer(t *testing.T, name, digest, sourceSHA string, calls *int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if calls != nil {
			*calls++
		}
		assert.Equal(t, githubAPIVersion, request.Header.Get("X-GitHub-Api-Version"))
		if request.URL.Path == "/repos/verity-org/verity/actions/runs/42/attempts/2" {
			writer.Header().Set("Content-Type", "application/json")
			response := exactRunAttemptResponse()
			response.ReferencedWorkflows[0].Path = "verity-org/verity/.github/workflows/build-verity-protected.yaml@refs/heads/main"
			require.NoError(t, json.NewEncoder(writer).Encode(response))
			return
		}
		assert.Equal(t, "/repos/verity-org/verity/actions/runs/42/artifacts", request.URL.Path)
		assert.Equal(t, "100", request.URL.Query().Get("per_page"))
		assert.Equal(t, name, request.URL.Query().Get("name"))
		writer.Header().Set("Content-Type", "application/json")
		_, err := fmt.Fprintf(writer,
			`{"total_count":1,"artifacts":[{"id":7,"name":%q,"expired":false,"digest":%q,"workflow_run":{"id":42,"head_sha":%q}}]}`,
			name, digest, sourceSHA)
		require.NoError(t, err)
	}))
}
