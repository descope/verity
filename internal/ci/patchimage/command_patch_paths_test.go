package patchimage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommand_platformRequested_writesBooleanOutcome(t *testing.T) {
	// Given
	outputPath := filepath.Join(t.TempDir(), "github-output")
	t.Setenv("GITHUB_OUTPUT", outputPath)

	// When
	err := NewCommand().Run(t.Context(), []string{
		"patch-image", "platform-requested", "--platforms", "linux/amd64,linux/arm64", "--platform", "arm64",
	})

	// Then
	require.NoError(t, err)
	assert.Equal(t, "enabled=true\n", readTextFile(t, outputPath))
}

func TestCommand_writePlatformMetrics_writesMetadataAndOutputPath(t *testing.T) {
	// Given
	runnerTemp := t.TempDir()
	outputPath := filepath.Join(t.TempDir(), "github-output")
	t.Setenv("GITHUB_OUTPUT", outputPath)

	// When
	err := NewCommand().Run(t.Context(), []string{
		"patch-image", "write-platform-metrics", "--arch", "amd64", "--duration", "17",
		"--exit-code", "0", "--digest", "sha256:sentinel", "--runner-temp", runnerTemp,
	})

	// Then
	require.NoError(t, err)
	path := filepath.Join(runnerTemp, "platform-amd64.json")
	var metrics PlatformMetrics
	require.NoError(t, json.Unmarshal([]byte(readTextFile(t, path)), &metrics))
	assert.Equal(t, "amd64", metrics.Arch)
	require.NotNil(t, metrics.DurationSeconds)
	require.NotNil(t, metrics.ExitCode)
	require.NotNil(t, metrics.StagingDigest)
	assert.Equal(t, int64(17), *metrics.DurationSeconds)
	assert.Equal(t, int64(0), *metrics.ExitCode)
	assert.Equal(t, "sha256:sentinel", *metrics.StagingDigest)
	assert.Equal(t, "path="+path+"\n", readTextFile(t, outputPath))
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestCommand_trivyDate_writesUTCDateKey(t *testing.T) {
	// Given
	outputPath := filepath.Join(t.TempDir(), "github-output")
	t.Setenv("GITHUB_OUTPUT", outputPath)

	// When
	err := NewCommand().Run(t.Context(), []string{"patch-image", "trivy-date"})

	// Then
	require.NoError(t, err)
	assert.Regexp(t, `^date=\d{4}-\d{2}-\d{2}-\d{2}\n$`, readTextFile(t, outputPath))
}

func TestCommand_emitPlatformSpan_invokesConfiguredOtelBinaryWithEvidence(t *testing.T) {
	// Given
	binDirectory := filepath.Join(t.TempDir(), "bin")
	argumentsPath := filepath.Join(t.TempDir(), "otel-args")
	otelPath := filepath.Join(binDirectory, "otel-cli")
	installFakeExecutable(t, binDirectory, "otel-cli", `printf '%s\n' "$*" > "$OTEL_ARGS"`)
	t.Setenv("OTEL_ARGS", argumentsPath)
	reportPath := filepath.Join(t.TempDir(), "pre.json")
	require.NoError(t, os.WriteFile(reportPath, []byte(fakePatchTrivyReport), 0o600))

	// When
	err := NewCommand().Run(t.Context(), []string{
		"patch-image", "emit-platform-span", "--image-name", "nginx", "--source-tag", "v1", "--platform", "linux/amd64",
		"--cve-before", "1", "--copa-exit", "0", "--copa-duration", "9", "--staging-digest", "sha256:stage",
		"--report", reportPath, "--otel-path", otelPath, "--home", t.TempDir(),
	})

	// Then
	require.NoError(t, err)
	arguments := readTextFile(t, argumentsPath)
	assert.Contains(t, arguments, "span --name patch-image.matrix --service verity-ci --kind internal --attrs")
	assert.Contains(t, arguments, "image=nginx")
	assert.Contains(t, arguments, "platform=linux/amd64")
	assert.Contains(t, arguments, "staging_digest=sha256:stage")
	assert.Contains(t, arguments, "package_list_sha256=")
}
