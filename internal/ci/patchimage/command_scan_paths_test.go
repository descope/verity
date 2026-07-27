package patchimage

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const fakePatchTrivyReport = `{"Results":[{"Vulnerabilities":[{"VulnerabilityID":"CVE-1","Severity":"CRITICAL","PkgName":"libc"}]}]}`

func TestCommand_workflowStart_fetchesTimestampAndWritesOutput(t *testing.T) {
	// Given
	binDirectory := filepath.Join(t.TempDir(), "bin")
	installFakeExecutable(t, binDirectory, "gh", `printf '%s\n' '{"run_started_at":"2026-07-25T11:12:13Z"}'`)
	prependCommandPath(t, binDirectory)
	outputPath := filepath.Join(t.TempDir(), "github-output")
	t.Setenv("GITHUB_OUTPUT", outputPath)

	// When
	err := NewCommand().Run(t.Context(), []string{
		"patch-image", "workflow-start", "--repository", "verity-org/verity", "--run-id", "42",
	})

	// Then
	require.NoError(t, err)
	assert.Equal(t, "value=2026-07-25T11:12:13Z\n", readTextFile(t, outputPath))
}

func TestCommand_downloadPreviousReport_decodesGitHubContent(t *testing.T) {
	// Given
	binDirectory := filepath.Join(t.TempDir(), "bin")
	encoded := base64.StdEncoding.EncodeToString([]byte(fakePatchTrivyReport))
	installFakeExecutable(t, binDirectory, "gh", `printf '%s\n' '{"content":"`+encoded+`"}'`)
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
	assert.Equal(t, fakePatchTrivyReport, readTextFile(t, destination))
	assert.Equal(t, "exists=true\n", readTextFile(t, outputPath))
}

func TestCommand_scanSource_runsTrivyAndWritesVulnerabilityCount(t *testing.T) {
	// Given
	binDirectory := filepath.Join(t.TempDir(), "bin")
	installFakeExecutable(t, binDirectory, "trivy", `
output=
while [ "$#" -gt 0 ]; do
  if [ "$1" = "--output" ]; then shift; output=$1; fi
  shift
done
printf '%s' '`+fakePatchTrivyReport+`' > "$output"`)
	prependCommandPath(t, binDirectory)
	reportPath := filepath.Join(t.TempDir(), "pre.json")
	outputPath := filepath.Join(t.TempDir(), "github-output")
	t.Setenv("GITHUB_OUTPUT", outputPath)

	// When
	err := NewCommand().Run(t.Context(), []string{
		"patch-image", "scan-source", "--image", "docker.io/library/nginx:v1", "--report", reportPath,
	})

	// Then
	require.NoError(t, err)
	assert.Equal(t, "vuln-count=1\n", readTextFile(t, outputPath))
	assert.Equal(t, fakePatchTrivyReport, readTextFile(t, reportPath))
}

func TestCommand_checkExisting_marksVulnerableImageForPatch(t *testing.T) {
	// Given
	binDirectory := filepath.Join(t.TempDir(), "bin")
	installFakeExecutable(t, binDirectory, "crane", "exit 0")
	installFakeExecutable(t, binDirectory, "trivy", `
output=
while [ "$#" -gt 0 ]; do
  if [ "$1" = "--output" ]; then shift; output=$1; fi
  shift
done
printf '%s' '`+fakePatchTrivyReport+`' > "$output"`)
	prependCommandPath(t, binDirectory)
	reportPath := filepath.Join(t.TempDir(), "existing.json")
	outputPath := filepath.Join(t.TempDir(), "github-output")
	t.Setenv("GITHUB_OUTPUT", outputPath)

	// When
	err := NewCommand().Run(t.Context(), []string{
		"patch-image", "check-existing", "--image-name", "nginx", "--source-tag", "v1",
		"--target-registry", "ghcr.io/verity", "--report", reportPath,
	})

	// Then
	require.NoError(t, err)
	assert.Equal(t, "needs-patch=true\n", readTextFile(t, outputPath))
	_, err = os.Stat(reportPath)
	require.NoError(t, err)
}
