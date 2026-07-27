package patchimage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCommand_registersEveryMigratedOperation(t *testing.T) {
	// Given
	wanted := []string{
		"parse-inputs", "trivy-date", "scan-source", "check-existing", "download-previous-report",
		"platform-requested", "write-platform-metrics", "install-otel", "emit-platform-span",
		"create-manifest", "scan-post", "compare-reports", "crane", "resolve-manifest", "cosign",
		"update-preflight", "merge-platform-metrics", "build-success-metrics", "workflow-start", "build-failure-metrics",
	}

	// When
	registered := make(map[string]bool)
	for _, command := range NewCommand().Commands {
		registered[command.Name] = true
	}

	// Then
	for _, name := range wanted {
		assert.True(t, registered[name], "missing patch-image command %q", name)
	}
}

func TestNewCommand_parseInputs_writesGitHubOutputs(t *testing.T) {
	// Given
	outputPath := filepath.Join(t.TempDir(), "github-output")
	t.Setenv("GITHUB_OUTPUT", outputPath)
	command := NewCommand()

	// When
	err := command.Run(t.Context(), []string{
		"patch-image", "parse-inputs",
		"--source-ref", "localhost:5000/nginx:v1@sha256:abc",
		"--image-name", "library/nginx", "--target-registry", "ghcr.io/verity",
	})

	// Then
	require.NoError(t, err)
	output, err := os.ReadFile(outputPath)
	require.NoError(t, err)
	assert.Equal(t, "source-tag=v1\nsafe-name=library-nginx\nstaging-registry=ghcr.io/verity/cache\n", string(output))
}
