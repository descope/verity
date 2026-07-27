package patchimage

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommand_mergePlatformMetrics_writesCompactedPlatformOutputs(t *testing.T) {
	// Given
	directory := t.TempDir()
	outputPath := filepath.Join(t.TempDir(), "github-output")
	require.NoError(t, os.WriteFile(filepath.Join(directory, "platform-amd64.json"), []byte("{\n  \"arch\": \"amd64\"\n}"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "platform-arm64.json"), []byte("broken"), 0o600))
	t.Setenv("GITHUB_OUTPUT", outputPath)

	// When
	err := NewCommand().Run(t.Context(), []string{"patch-image", "merge-platform-metrics", "--directory", directory})

	// Then
	require.NoError(t, err)
	assert.Equal(t, "amd64={\"arch\":\"amd64\"}\narm64=null\n", readTextFile(t, outputPath))
}

func TestCommand_buildFailureMetrics_writesDocumentAndWorkflowOutputs(t *testing.T) {
	// Given
	workingDirectory := useTempWorkingDirectory(t)
	platformDirectory := t.TempDir()
	outputPath := filepath.Join(t.TempDir(), "github-output")
	require.NoError(t, os.WriteFile(filepath.Join(platformDirectory, "platform-amd64.json"), []byte(`{"arch":"amd64","copa_exit_code":1}`), 0o600))
	t.Setenv("GITHUB_OUTPUT", outputPath)

	// When
	err := NewCommand().Run(t.Context(), []string{
		"patch-image", "build-failure-metrics",
		"--image-name", "library/nginx", "--source-tag", "v1", "--safe-name", "library-nginx",
		"--run-id", "41", "--run-attempt", "2", "--source-sha", "sentinel-sha",
		"--started-at", "2026-07-25T10:00:00Z", "--platform-directory", platformDirectory,
	})

	// Then
	require.NoError(t, err)
	filename := "metrics-library-nginx-v1.json"
	document := readMetricsDocument(t, filepath.Join(workingDirectory, filename))
	assert.Equal(t, int64(41), document.Run.ID)
	assert.Equal(t, int64(2), document.Run.Attempt)
	assert.Equal(t, "failure", document.Run.Conclusion)
	assert.Equal(t, "library/nginx", document.Image.Name)
	assert.JSONEq(t, `{"arch":"amd64","copa_exit_code":1}`, string(document.Platforms.AMD64))
	assert.Equal(t, json.RawMessage("null"), document.Platforms.ARM64)
	assert.Equal(t, "filename="+filename+"\nartifact-name=metrics-library-nginx-v1\nrun-id=41\nrun-attempt=2\nsource-sha=sentinel-sha\n", readTextFile(t, outputPath))
}

func TestCommand_buildFailureMetrics_rejectsInvalidRunIDBeforeWriting(t *testing.T) {
	// Given
	workingDirectory := useTempWorkingDirectory(t)

	// When
	err := NewCommand().Run(t.Context(), []string{
		"patch-image", "build-failure-metrics",
		"--safe-name", "library-nginx", "--run-id", "0", "--run-attempt", "2",
		"--source-sha", "sentinel-sha", "--started-at", "2026-07-25T10:00:00Z",
		"--platform-directory", t.TempDir(),
	})

	// Then
	require.ErrorIs(t, err, ErrInvalidCommandInput)
	_, statErr := os.Stat(filepath.Join(workingDirectory, "metrics-library-nginx-.json"))
	assert.True(t, errors.Is(statErr, os.ErrNotExist))
}

func TestCommand_buildSuccessMetrics_combinesReportsOutcomesAndSupplyChain(t *testing.T) {
	// Given
	workingDirectory := useTempWorkingDirectory(t)
	binDirectory := filepath.Join(t.TempDir(), "bin")
	installFakeExecutable(t, binDirectory, "gh", `printf '%s\n' '{"run_started_at":"2026-07-25T10:00:00Z"}'`)
	prependCommandPath(t, binDirectory)
	preReport := filepath.Join(t.TempDir(), "pre.json")
	postReport := filepath.Join(t.TempDir(), "missing-post.json")
	sbomPath := filepath.Join(t.TempDir(), "sbom.json")
	outputPath := filepath.Join(t.TempDir(), "github-output")
	preContent := []byte(`{"Results":[{"Vulnerabilities":[{"VulnerabilityID":"CVE-1","Severity":"HIGH","PkgName":"openssl"}]}]}`)
	sbomContent := []byte(`{"bomFormat":"CycloneDX"}`)
	require.NoError(t, os.WriteFile(preReport, preContent, 0o600))
	require.NoError(t, os.WriteFile(sbomPath, sbomContent, 0o600))
	t.Setenv("GITHUB_OUTPUT", outputPath)

	// When
	err := NewCommand().Run(t.Context(), []string{
		"patch-image", "build-success-metrics",
		"--repository", "verity-org/verity", "--image-name", "library/nginx", "--source-tag", "v1",
		"--safe-name", "library-nginx", "--run-id", "42", "--run-attempt", "3", "--source-sha", "sentinel-sha",
		"--target-ref", "ghcr.io/verity/nginx:v1", "--manifest-digest", "sha256:manifest",
		"--vuln-after", "4", "--pre-report", preReport, "--post-report", postReport,
		"--amd64", `{"arch":"amd64"}`, "--arm64", "broken-json", "--push-outcome", "failure",
		"--rekor-url", "https://rekor.example/entry", "--attestation-id", "attestation-1",
		"--sbom", sbomPath, "--attestation-bundle-path", "bundle.json",
	})

	// Then
	require.NoError(t, err)
	document := readMetricsDocument(t, filepath.Join(workingDirectory, "metrics-library-nginx-v1.json"))
	assert.Equal(t, "2026-07-25T10:00:00Z", document.Run.StartedAt)
	assert.Equal(t, "failure", document.Run.Conclusion)
	require.NotNil(t, document.Scan.Before)
	require.NotNil(t, document.Scan.After)
	assert.Equal(t, 1, document.Scan.Before.VulnerabilityCount)
	assert.Equal(t, 4, document.Scan.After.VulnerabilityCount)
	assert.Equal(t, 1, document.Scan.Before.BySeverity["HIGH"])
	assert.JSONEq(t, `{"arch":"amd64"}`, string(document.Platforms.AMD64))
	assert.Equal(t, json.RawMessage("null"), document.Platforms.ARM64)
	digest := sha256.Sum256(sbomContent)
	require.NotNil(t, document.SupplyChain.SBOMDigest)
	assert.Equal(t, "sha256:"+hex.EncodeToString(digest[:]), *document.SupplyChain.SBOMDigest)
	assert.Contains(t, readTextFile(t, outputPath), "artifact-name=metrics-library-nginx-v1\n")
}
