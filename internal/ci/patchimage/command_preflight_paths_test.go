package patchimage

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommand_updatePreflight_buildsManifestPayloadFromDigestAndReport(t *testing.T) {
	// Given
	binDirectory := filepath.Join(t.TempDir(), "bin")
	installFakeExecutable(t, binDirectory, "crane", `printf '%s\n' 'sha256:upstream'`)
	payloadPath := filepath.Join(t.TempDir(), "payload.json")
	installFakeExecutable(t, binDirectory, "gh", `
if [ "$2" = "--method" ]; then cat > "$GH_CAPTURE"; exit 0; fi
printf '%s\n' '{"message":"Not Found"}' >&2
exit 1`)
	prependCommandPath(t, binDirectory)
	t.Setenv("GH_CAPTURE", payloadPath)
	reportPath := filepath.Join(t.TempDir(), "post.json")
	require.NoError(t, os.WriteFile(reportPath, []byte(fakePatchTrivyReport), 0o600))

	// When
	err := NewCommand().Run(t.Context(), []string{
		"patch-image", "update-preflight", "--repository", "verity-org/verity", "--image-name", "nginx",
		"--source-tag", "v1", "--source-ref", "docker.io/library/nginx:v1", "--post-report", reportPath,
		"--attempts", "1", "--retry-delay", "0s",
	})

	// Then
	require.NoError(t, err)
	var payload struct {
		Content string `json:"content"`
		Branch  string `json:"branch"`
	}
	require.NoError(t, json.Unmarshal([]byte(readTextFile(t, payloadPath)), &payload))
	assert.Equal(t, "reports", payload.Branch)
	decoded, err := base64.StdEncoding.DecodeString(payload.Content)
	require.NoError(t, err)
	var manifest map[string]struct {
		UpstreamDigest         string `json:"upstream_digest"`
		PatchedVulnerabilities int    `json:"patched_vulns"`
	}
	require.NoError(t, json.Unmarshal(decoded, &manifest))
	assert.Equal(t, "sha256:upstream", manifest["nginx/v1"].UpstreamDigest)
	assert.Equal(t, 1, manifest["nginx/v1"].PatchedVulnerabilities)
}

func TestCommand_updatePreflight_returnsSuccessAfterExhaustedBestEffortUpdate(t *testing.T) {
	// Given
	binDirectory := filepath.Join(t.TempDir(), "bin")
	installFakeExecutable(t, binDirectory, "crane", "exit 1")
	installFakeExecutable(t, binDirectory, "gh", `printf '%s\n' 'transient failure' >&2; exit 1`)
	prependCommandPath(t, binDirectory)

	// When
	err := NewCommand().Run(t.Context(), []string{
		"patch-image", "update-preflight", "--repository", "verity-org/verity", "--image-name", "nginx",
		"--source-tag", "v1", "--source-ref", "docker.io/library/nginx:v1", "--post-report", filepath.Join(t.TempDir(), "missing.json"),
		"--attempts", "1", "--retry-delay", "0s",
	})

	// Then
	require.NoError(t, err)
}
