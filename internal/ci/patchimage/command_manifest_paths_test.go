package patchimage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommand_createManifest_checksPlatformsAndWritesManifestTag(t *testing.T) {
	// Given
	binDirectory := filepath.Join(t.TempDir(), "bin")
	dockerArgsPath := filepath.Join(t.TempDir(), "docker-args")
	installFakeExecutable(t, binDirectory, "crane", `printf '%s\n' 'sha256:sentinel'`)
	installFakeExecutable(t, binDirectory, "docker", `printf '%s\n' "$*" > "$DOCKER_ARGS"`)
	prependCommandPath(t, binDirectory)
	t.Setenv("DOCKER_ARGS", dockerArgsPath)
	outputPath := filepath.Join(t.TempDir(), "github-output")
	t.Setenv("GITHUB_OUTPUT", outputPath)

	// When
	err := NewCommand().Run(t.Context(), []string{
		"patch-image", "create-manifest", "--image-name", "library/nginx", "--source-tag", "v1",
		"--staging-registry", "ghcr.io/verity/cache", "--platforms", "linux/amd64,linux/arm64",
	})

	// Then
	require.NoError(t, err)
	assert.Equal(t, "manifest-tag=ghcr.io/verity/cache:library-nginx-v1\n", readTextFile(t, outputPath))
	assert.Contains(t, readTextFile(t, dockerArgsPath), "buildx imagetools create --tag ghcr.io/verity/cache:library-nginx-v1")
	assert.Contains(t, readTextFile(t, dockerArgsPath), "ghcr.io/verity/cache:library-nginx-v1-amd64")
	assert.Contains(t, readTextFile(t, dockerArgsPath), "ghcr.io/verity/cache:library-nginx-v1-arm64")
}

func TestCommand_compareReports_skipsPublishWhenEvidenceAndTargetMatch(t *testing.T) {
	// Given
	binDirectory := filepath.Join(t.TempDir(), "bin")
	installFakeExecutable(t, binDirectory, "crane", "exit 0")
	prependCommandPath(t, binDirectory)
	directory := t.TempDir()
	currentReport := filepath.Join(directory, "current.json")
	previousReport := filepath.Join(directory, "previous.json")
	require.NoError(t, os.WriteFile(currentReport, []byte(fakePatchTrivyReport), 0o600))
	require.NoError(t, os.WriteFile(previousReport, []byte(fakePatchTrivyReport), 0o600))
	outputPath := filepath.Join(t.TempDir(), "github-output")
	t.Setenv("GITHUB_OUTPUT", outputPath)

	// When
	err := NewCommand().Run(t.Context(), []string{
		"patch-image", "compare-reports", "--image-name", "nginx", "--source-tag", "v1",
		"--target-registry", "ghcr.io/verity", "--current-report", currentReport,
		"--previous-report", previousReport, "--previous-existed",
	})

	// Then
	require.NoError(t, err)
	assert.Equal(t, "changed=false\n", readTextFile(t, outputPath))
}

func TestCommand_craneCopy_writesManifestAndPublishingOutputs(t *testing.T) {
	// Given
	binDirectory := filepath.Join(t.TempDir(), "bin")
	installFakeExecutable(t, binDirectory, "crane", `
if [ "$1" = "digest" ]; then printf '%s\n' 'sha256:final'; fi`)
	prependCommandPath(t, binDirectory)
	directory := t.TempDir()
	manifestPath := filepath.Join(directory, "final-manifest.txt")
	outputPath := filepath.Join(directory, "github-output")
	envPath := filepath.Join(directory, "github-env")
	t.Setenv("GITHUB_OUTPUT", outputPath)
	t.Setenv("GITHUB_ENV", envPath)

	// When
	err := NewCommand().Run(t.Context(), []string{
		"patch-image", "crane", "copy", "--image-name", "nginx", "--source-tag", "v1",
		"--target-registry", "ghcr.io/verity", "--manifest-tag", "ghcr.io/verity/cache:nginx-v1",
		"--manifest-file", manifestPath,
	})

	// Then
	require.NoError(t, err)
	assert.Equal(t, "ghcr.io/verity/nginx:v1\n", readTextFile(t, manifestPath))
	assert.Equal(t, "digest=sha256:final\nfinal-tag=ghcr.io/verity/nginx:v1\nfinal-repo=ghcr.io/verity/nginx\n", readTextFile(t, outputPath))
	assert.Equal(t, "DIGEST=sha256:final\nFINAL_TAG=ghcr.io/verity/nginx:v1\nFINAL_REPO=ghcr.io/verity/nginx\n", readTextFile(t, envPath))
}

func TestCommand_resolveManifest_writesResolvedDigestAndTag(t *testing.T) {
	// Given
	binDirectory := filepath.Join(t.TempDir(), "bin")
	installFakeExecutable(t, binDirectory, "crane", `printf '%s\n' 'sha256:resolved'`)
	prependCommandPath(t, binDirectory)
	outputPath := filepath.Join(t.TempDir(), "github-output")
	t.Setenv("GITHUB_OUTPUT", outputPath)

	// When
	err := NewCommand().Run(t.Context(), []string{
		"patch-image", "resolve-manifest", "--image-name", "nginx", "--source-tag", "v1", "--target-registry", "ghcr.io/verity",
	})

	// Then
	require.NoError(t, err)
	assert.Equal(t, "digest=sha256:resolved\nfinal-tag=ghcr.io/verity/nginx:v1\n", readTextFile(t, outputPath))
}

func TestCommand_cosignSign_extractsRekorURLFromGeneratedBundle(t *testing.T) {
	// Given
	binDirectory := filepath.Join(t.TempDir(), "bin")
	installFakeExecutable(t, binDirectory, "cosign", `
bundle=
while [ "$#" -gt 0 ]; do
  if [ "$1" = "--bundle" ]; then shift; bundle=$1; fi
  shift
done
printf '%s' '{"logIndex":77}' > "$bundle"
printf '%s\n' 'signed'
printf '%s\n' 'verified' >&2`)
	prependCommandPath(t, binDirectory)
	directory := t.TempDir()
	bundlePath := filepath.Join(directory, "bundle.json")
	cosignOutputPath := filepath.Join(directory, "cosign-output")
	workflowOutputPath := filepath.Join(directory, "github-output")
	t.Setenv("GITHUB_OUTPUT", workflowOutputPath)

	// When
	err := NewCommand().Run(t.Context(), []string{
		"patch-image", "cosign", "sign", "--final-repo", "ghcr.io/verity/nginx", "--digest", "sha256:final",
		"--bundle", bundlePath, "--output", cosignOutputPath,
	})

	// Then
	require.NoError(t, err)
	assert.Equal(t, "signed\nverified\n", readTextFile(t, cosignOutputPath))
	assert.Equal(t, "rekor-url=https://rekor.sigstore.dev/api/v1/log/entries?logIndex=77\n", readTextFile(t, workflowOutputPath))
}
