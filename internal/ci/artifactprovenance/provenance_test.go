package artifactprovenance

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testRepository    = "verity-org/verity"
	testSourceSHA     = "0123456789abcdef0123456789abcdef01234567"
	testPublicationID = "publication-42"
	testArtifactName  = "chart-publication-publication-42"
	testDigest        = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
)

func TestVerifyDownloaded_accepts_exact_API_and_manifest_identity(t *testing.T) {
	// Given exact expected identity, manifest, run metadata, and artifact metadata.
	identity := testIdentity(t)
	manifestPath := filepath.Join(t.TempDir(), "producer-manifest.json")
	require.NoError(t, WriteManifest(manifestPath, &identity))
	server := newGitHubFixture(t)

	// When downloaded provenance is verified.
	err := VerifyDownloaded(t.Context(), &VerifyOptions{
		Identity: identity, ArtifactDigest: testDigest, ManifestPath: manifestPath,
		Token: "token", APIBaseURL: server.URL, HTTPClient: server.Client(),
	})

	// Then repository, run attempt, source, publication, name, and digest all match.
	require.NoError(t, err)
}

func TestVerifyDownloaded_rejects_wrong_or_stale_provenance(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*IdentityInput, *string)
	}{
		{name: "foreign repository", mutate: func(input *IdentityInput, _ *string) {
			input.Repository = "other-org/other-repo"
		}},
		{name: "wrong run attempt", mutate: func(input *IdentityInput, _ *string) {
			input.RunAttempt = 999
		}},
		{name: "wrong source", mutate: func(input *IdentityInput, _ *string) {
			input.SourceSHA = strings.Repeat("b", 40)
		}},
		{name: "wrong publication", mutate: func(input *IdentityInput, _ *string) {
			input.PublicationID = "publication-wrong"
		}},
		{name: "wrong artifact name", mutate: func(input *IdentityInput, _ *string) {
			input.ArtifactName = "wrong-artifact"
		}},
		{name: "wrong digest", mutate: func(_ *IdentityInput, digest *string) {
			*digest = "sha256:" + strings.Repeat("b", 64)
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given a valid manifest and one mismatched expected field.
			manifestIdentity := testIdentity(t)
			manifestPath := filepath.Join(t.TempDir(), "producer-manifest.json")
			require.NoError(t, WriteManifest(manifestPath, &manifestIdentity))
			input := testIdentityInput()
			digest := testDigest
			test.mutate(&input, &digest)
			expected, err := ParseIdentity(&input)
			require.NoError(t, err)
			server := newGitHubFixture(t)

			// When downloaded provenance is verified.
			err = VerifyDownloaded(t.Context(), &VerifyOptions{
				Identity: expected, ArtifactDigest: digest, ManifestPath: manifestPath,
				Token: "token", APIBaseURL: server.URL, HTTPClient: server.Client(),
			})

			// Then the mismatch fails closed.
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrProvenanceMismatch)
		})
	}
}

func TestVerifyDownloaded_rejects_stale_API_attempt_and_digest(t *testing.T) {
	tests := []struct {
		name    string
		fixture githubFixture
	}{
		{name: "stale run attempt", fixture: githubFixture{runAttempt: 2, digest: testDigest}},
		{name: "wrong artifact digest", fixture: githubFixture{runAttempt: 3, digest: "sha256:" + strings.Repeat("b", 64)}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given a matching manifest but stale or wrong GitHub API metadata.
			identity := testIdentity(t)
			manifestPath := filepath.Join(t.TempDir(), "producer-manifest.json")
			require.NoError(t, WriteManifest(manifestPath, &identity))
			server := newGitHubFixtureWith(t, test.fixture)

			// When downloaded provenance is verified.
			err := VerifyDownloaded(t.Context(), &VerifyOptions{
				Identity: identity, ArtifactDigest: testDigest, ManifestPath: manifestPath,
				Token: "token", APIBaseURL: server.URL, HTTPClient: server.Client(),
			})

			// Then stale attempt or digest metadata fails closed.
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrProvenanceMismatch)
		})
	}
}

func TestVerifyDownloaded_rejects_manifest_without_run_attempt(t *testing.T) {
	// Given a downloaded manifest with the required run-attempt field omitted.
	manifestPath := filepath.Join(t.TempDir(), "producer-manifest.json")
	require.NoError(t, os.WriteFile(manifestPath, []byte(
		`{"version":1,"repository":"verity-org/verity","run_id":42,`+
			`"source_sha":"0123456789abcdef0123456789abcdef01234567",`+
			`"publication_id":"publication-42","artifact_name":"chart-publication-publication-42"}`,
	), 0o600))
	identity := testIdentity(t)
	server := newGitHubFixture(t)

	// When downloaded provenance is verified.
	err := VerifyDownloaded(t.Context(), &VerifyOptions{
		Identity: identity, ArtifactDigest: testDigest, ManifestPath: manifestPath,
		Token: "token", APIBaseURL: server.URL, HTTPClient: server.Client(),
	})

	// Then the incomplete manifest fails before artifact use.
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidProvenance)
}

func testIdentity(t *testing.T) Identity {
	t.Helper()
	input := testIdentityInput()
	identity, err := ParseIdentity(&input)
	require.NoError(t, err)
	return identity
}

func testIdentityInput() IdentityInput {
	return IdentityInput{
		Repository: testRepository, RunID: 42, RunAttempt: 3,
		SourceSHA: testSourceSHA, PublicationID: testPublicationID, ArtifactName: testArtifactName,
	}
}

func newGitHubFixture(t *testing.T) *httptest.Server {
	return newGitHubFixtureWith(t, githubFixture{runAttempt: 3, digest: testDigest})
}

type githubFixture struct {
	runAttempt uint64
	digest     string
}

func newGitHubFixtureWith(t *testing.T, fixture githubFixture) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		require.Equal(t, "Bearer token", request.Header.Get("Authorization"))
		switch request.URL.Path {
		case "/repos/verity-org/verity/actions/runs/42":
			_, _ = fmt.Fprintf(response, `{"id":42,"run_attempt":%d,"head_sha":%q,"repository":{"full_name":%q}}`, fixture.runAttempt, testSourceSHA, testRepository)
		case "/repos/verity-org/verity/actions/runs/42/artifacts":
			require.Equal(t, testArtifactName, request.URL.Query().Get("name"))
			_, _ = fmt.Fprintf(response, `{"total_count":1,"artifacts":[{"id":7,"name":%q,"digest":%q,"expired":false,"workflow_run":{"id":42,"head_sha":%q}}]}`, testArtifactName, fixture.digest, testSourceSHA)
		default:
			http.NotFound(response, request)
		}
	}))
	t.Cleanup(server.Close)
	return server
}
