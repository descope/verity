package patchimage

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/ci/workflowops/retry"
)

func TestManifestService_Create_checksEverySourceBeforeDockerCreate(t *testing.T) {
	// Given
	runner := &fakeRunner{results: []runnerResult{
		{result: retry.Result{Stdout: []byte("sha256:amd64")}},
		{result: retry.Result{Stdout: []byte("sha256:arm64")}},
		{result: retry.Result{}},
	}}
	service := ManifestService{Runner: runner}

	// When
	result, err := service.Create(t.Context(), ManifestPlanInput{
		ImageName: "nginx", SourceTag: "1.29.3", StagingRegistry: "ghcr.io/verity/cache",
		Platforms: "linux/amd64,linux/arm64",
	})

	// Then
	require.NoError(t, err)
	assert.Equal(t, "ghcr.io/verity/cache:nginx-1.29.3", result.ManifestTag)
	require.Len(t, runner.calls, 3)
	assert.Equal(t, "crane", runner.calls[0].Name)
	assert.Equal(t, "crane", runner.calls[1].Name)
	assert.Equal(t, "docker", runner.calls[2].Name)
	assert.Equal(t, []string{"buildx", "imagetools", "create", "--tag", result.ManifestTag, result.SourceTags[0], result.SourceTags[1]}, runner.calls[2].Args)
}

func TestManifestService_Copy_writesFinalManifestAndReturnsDigest(t *testing.T) {
	// Given
	directory := t.TempDir()
	runner := &fakeRunner{results: []runnerResult{
		{result: retry.Result{}},
		{result: retry.Result{Stdout: []byte("sha256:final\n")}},
	}}
	service := ManifestService{Runner: runner}

	// When
	result, err := service.Copy(t.Context(), &CopyManifestInput{
		ManifestTag: "ghcr.io/verity/cache:nginx-1.29.3", TargetRegistry: "ghcr.io/verity",
		ImageName: "nginx", SourceTag: "1.29.3", ManifestFile: filepath.Join(directory, "final-manifest.txt"),
	})

	// Then
	require.NoError(t, err)
	assert.Equal(t, "ghcr.io/verity/nginx:1.29.3", result.FinalTag)
	assert.Equal(t, "sha256:final", result.Digest)
	written, err := os.ReadFile(filepath.Join(directory, "final-manifest.txt"))
	require.NoError(t, err)
	assert.Equal(t, "ghcr.io/verity/nginx:1.29.3\n", string(written))
}

func TestManifestService_Sign_extractsBundleRekorURLAndCapturesOutput(t *testing.T) {
	// Given
	directory := t.TempDir()
	bundlePath := filepath.Join(directory, "bundle.json")
	outputPath := filepath.Join(directory, "cosign-output")
	runner := &fakeRunner{run: func(_ context.Context, command *retry.Command) (retry.Result, error) {
		require.NoError(t, os.WriteFile(bundlePath, []byte(`{"logIndex":77}`), 0o600))
		assert.Equal(t, "cosign", command.Name)
		return retry.Result{Stdout: []byte("signed\n"), Stderr: []byte("verified\n")}, nil
	}}

	// When
	result, err := (ManifestService{Runner: runner}).Sign(t.Context(), SignManifestInput{
		Reference: "ghcr.io/verity/nginx@sha256:final", BundlePath: bundlePath, OutputPath: outputPath,
	})

	// Then
	require.NoError(t, err)
	assert.Equal(t, "https://rekor.sigstore.dev/api/v1/log/entries?logIndex=77", result.RekorURL)
	output, err := os.ReadFile(outputPath)
	require.NoError(t, err)
	assert.Equal(t, "signed\nverified\n", string(output))
}
