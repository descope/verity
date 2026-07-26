package patchimage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommand_buildSuccessMetrics_rejectsInvalidRunAttemptBeforeFetching(t *testing.T) {
	// Given
	useTempWorkingDirectory(t)

	// When
	err := NewCommand().Run(t.Context(), []string{
		"patch-image", "build-success-metrics", "--safe-name", "nginx", "--run-id", "42",
		"--run-attempt", "0", "--source-sha", "sentinel-sha",
	})

	// Then
	require.ErrorIs(t, err, ErrInvalidCommandInput)
}

func TestCommand_buildSuccessMetrics_returnsSBOMReadFailure(t *testing.T) {
	// Given
	workingDirectory := useTempWorkingDirectory(t)
	binDirectory := filepath.Join(t.TempDir(), "bin")
	installFakeExecutable(t, binDirectory, "gh", `printf '%s\n' '{"run_started_at":"2026-07-25T10:00:00Z"}'`)
	prependCommandPath(t, binDirectory)

	// When
	err := NewCommand().Run(t.Context(), []string{
		"patch-image", "build-success-metrics", "--repository", "verity-org/verity", "--safe-name", "nginx",
		"--source-tag", "v1", "--run-id", "42", "--run-attempt", "1", "--source-sha", "sentinel-sha",
		"--sbom", t.TempDir(),
	})

	// Then
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read digest input")
	_, statErr := os.Stat(filepath.Join(workingDirectory, "metrics-nginx-v1.json"))
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestCommand_downloadPreviousReport_writesFallbackWhenGitHubFails(t *testing.T) {
	// Given
	binDirectory := filepath.Join(t.TempDir(), "bin")
	installFakeExecutable(t, binDirectory, "gh", `printf '%s\n' 'upstream unavailable' >&2; exit 1`)
	prependCommandPath(t, binDirectory)
	destination := filepath.Join(t.TempDir(), "previous.json")
	outputPath := filepath.Join(t.TempDir(), "github-output")
	t.Setenv("GITHUB_OUTPUT", outputPath)

	// When
	err := NewCommand().Run(t.Context(), []string{
		"patch-image", "download-previous-report", "--repository", "verity-org/verity",
		"--image-name", "nginx", "--source-tag", "v1", "--destination", destination,
	})

	// Then
	require.NoError(t, err)
	assert.Equal(t, `{"Results":[]}`, readTextFile(t, destination))
	assert.Equal(t, "exists=false\n", readTextFile(t, outputPath))
}

func TestCommand_scanPost_returnsTrivyFailureWithoutOutput(t *testing.T) {
	// Given
	binDirectory := filepath.Join(t.TempDir(), "bin")
	installFakeExecutable(t, binDirectory, "trivy", `printf '%s\n' 'scan failed' >&2; exit 1`)
	prependCommandPath(t, binDirectory)
	outputPath := filepath.Join(t.TempDir(), "github-output")
	t.Setenv("GITHUB_OUTPUT", outputPath)

	// When
	err := NewCommand().Run(t.Context(), []string{
		"patch-image", "scan-post", "--image", "docker.io/library/nginx:v1", "--report", filepath.Join(t.TempDir(), "post.json"),
	})

	// Then
	require.Error(t, err)
	assert.Contains(t, err.Error(), "scan image")
	_, statErr := os.Stat(outputPath)
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}
