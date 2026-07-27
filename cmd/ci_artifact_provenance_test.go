package cmd

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

func TestCIArtifactProvenanceCommand_writes_then_verifies_exact_download(t *testing.T) {
	// Given the public command, exact identity, and matching GitHub API metadata.
	manifestPath := filepath.Join(t.TempDir(), "producer-manifest.json")
	server := newArtifactGitHubFixture(t)
	stdout := &bytes.Buffer{}
	root := &cli.Command{
		Writer: stdout,
		Commands: []*cli.Command{{
			Name: "ci", Commands: []*cli.Command{newCIArtifactProvenanceCommand()},
		}},
	}
	identityArgs := []string{
		"--repository", "verity-org/verity",
		"--run-id", "42",
		"--run-attempt", "3",
		"--source-sha", "0123456789abcdef0123456789abcdef01234567",
		"--publication-id", "publication-42",
		"--artifact-name", "chart-publication-publication-42",
		"--manifest", manifestPath,
	}

	// When the producer writes the manifest and the consumer verifies the download.
	writeArgs := append([]string{"verity", "ci", "artifact-provenance", "write-manifest"}, identityArgs...)
	require.NoError(t, root.Run(t.Context(), writeArgs))
	verifyArgs := append([]string{"verity", "ci", "artifact-provenance", "verify-download"}, identityArgs...)
	verifyArgs = append(
		verifyArgs,
		"--artifact-digest", "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"--token", "token",
		"--api-base-url", server.URL,
	)
	require.NoError(t, root.Run(t.Context(), verifyArgs))

	// Then both typed boundaries report success.
	assert.Contains(t, stdout.String(), "wrote artifact provenance manifest")
	assert.Contains(t, stdout.String(), "verified exact artifact provenance")
}

func newArtifactGitHubFixture(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/repos/verity-org/verity/actions/runs/42":
			_, _ = fmt.Fprint(response, `{"id":42,"run_attempt":3,"head_sha":"0123456789abcdef0123456789abcdef01234567","repository":{"full_name":"verity-org/verity"}}`)
		case "/repos/verity-org/verity/actions/runs/42/artifacts":
			_, _ = fmt.Fprint(response, `{"total_count":1,"artifacts":[{"id":7,"name":"chart-publication-publication-42","digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","expired":false,"workflow_run":{"id":42,"head_sha":"0123456789abcdef0123456789abcdef01234567"}}]}`)
		default:
			http.NotFound(response, request)
		}
	}))
	t.Cleanup(server.Close)
	return server
}
