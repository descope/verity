package buildmetadata

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testArtifactDigest = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

func TestVerifyCurrentRunArtifact_accepts_exact_current_run_identity(t *testing.T) {
	// Given GitHub returns one current-run artifact with exact name, digest, and source.
	artifactName := "verity-linux-amd64-" + testBuildKey
	server := newArtifactAPIServer(t, artifactAPIResponse(artifactName, testArtifactDigest, testSourceSHA, false))

	// When the remote identity is verified.
	err := VerifyCurrentRunArtifact(context.Background(), RemoteVerifyOptions{
		APIBaseURL: server.URL, Token: "token", Repository: "verity-org/verity", RunID: 42,
		ArtifactName: artifactName, ArtifactDigest: testArtifactDigest, SourceSHA: testSourceSHA,
	})

	// Then the current-run artifact is accepted.
	require.NoError(t, err)
}

func TestVerifyCurrentRunArtifact_rejects_wrong_artifact_digest_source_and_repository(t *testing.T) {
	artifactName := "verity-linux-amd64-" + testBuildKey
	tests := []struct {
		name     string
		response string
		mutate   func(*RemoteVerifyOptions)
	}{
		{name: "wrong artifact name", response: artifactAPIResponse("verity-linux-amd64-"+strings.Repeat("c", 64), testArtifactDigest, testSourceSHA, false)},
		{name: "wrong digest", response: artifactAPIResponse(artifactName, "sha256:"+strings.Repeat("c", 64), testSourceSHA, false)},
		{name: "wrong source", response: artifactAPIResponse(artifactName, testArtifactDigest, strings.Repeat("c", 40), false)},
		{name: "expired artifact", response: artifactAPIResponse(artifactName, testArtifactDigest, testSourceSHA, true)},
		{name: "wrong protected repository", response: artifactAPIResponse(artifactName, testArtifactDigest, testSourceSHA, false), mutate: func(options *RemoteVerifyOptions) {
			options.Repository = "fork/verity"
			options.ProtectedAttestation = true
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given one hostile current-run artifact identity.
			server := newArtifactAPIServer(t, test.response)
			options := RemoteVerifyOptions{
				APIBaseURL: server.URL, Token: "token", Repository: "verity-org/verity", RunID: 42,
				ArtifactName: artifactName, ArtifactDigest: testArtifactDigest, SourceSHA: testSourceSHA,
			}
			if test.mutate != nil {
				test.mutate(&options)
			}

			// When remote verification runs.
			err := VerifyCurrentRunArtifact(context.Background(), options)

			// Then the mismatch is rejected before artifact download.
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrArtifactMismatch)
		})
	}
}

func newArtifactAPIServer(t *testing.T, response string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assert.Equal(t, "/repos/verity-org/verity/actions/runs/42/artifacts", request.URL.Path)
		assert.Equal(t, "Bearer token", request.Header.Get("Authorization"))
		assert.Equal(t, "application/vnd.github+json", request.Header.Get("Accept"))
		writer.Header().Set("Content-Type", "application/json")
		_, err := writer.Write([]byte(response))
		require.NoError(t, err)
	}))
}

func artifactAPIResponse(name, digest, sourceSHA string, expired bool) string {
	return fmt.Sprintf(`{"total_count":1,"artifacts":[{"id":7,"name":%q,"expired":%t,"digest":%q,"workflow_run":{"id":42,"head_sha":%q}}]}`,
		name, expired, digest, sourceSHA)
}
