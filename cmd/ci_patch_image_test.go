package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

func TestCIPatchImageCommand_registersEveryMigratedWorkflowOperation(t *testing.T) {
	// Given
	wanted := []string{
		"parse-inputs", "trivy-date", "scan-source", "check-existing", "download-previous-report",
		"platform-requested", "write-platform-metrics", "install-otel", "emit-platform-span",
		"create-manifest", "scan-post", "compare-reports", "crane", "resolve-manifest", "cosign",
		"update-preflight", "merge-platform-metrics", "build-success-metrics", "workflow-start", "build-failure-metrics",
	}

	// When
	registered := make(map[string]bool)
	for _, command := range ciPatchImageCommand.Commands {
		registered[command.Name] = true
	}

	// Then
	for _, name := range wanted {
		assert.True(t, registered[name], "missing patch-image command %q", name)
	}
}

func TestCIPatchImageCommand_parseInputs_writesGitHubOutputs(t *testing.T) {
	// Given
	outputPath := filepath.Join(t.TempDir(), "github-output")
	t.Setenv("GITHUB_OUTPUT", outputPath)
	root := &cli.Command{Commands: []*cli.Command{CICommand}}

	// When
	err := root.Run(t.Context(), []string{
		"verity", "ci", "patch-image", "parse-inputs",
		"--source-ref", "localhost:5000/nginx:v1@sha256:abc",
		"--image-name", "library/nginx", "--target-registry", "ghcr.io/verity",
	})

	// Then
	require.NoError(t, err)
	output, err := os.ReadFile(outputPath)
	require.NoError(t, err)
	assert.Equal(t, "source-tag=v1\nsafe-name=library-nginx\nstaging-registry=ghcr.io/verity/cache\n", string(output))
}

func TestPatchImageWorkflow_routesOwnedLogicThroughTypedCommands(t *testing.T) {
	// Given
	workflow, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "patch-image.yaml"))
	require.NoError(t, err)
	text := string(workflow)

	// When / Then
	for _, invocation := range []string{
		"./verity ci patch-image parse-inputs",
		"./verity ci patch-image trivy-date",
		"./verity ci patch-image scan-source",
		"./verity ci patch-image check-existing",
		"./verity ci patch-image download-previous-report",
		"./verity ci patch-image platform-requested",
		"./verity ci patch-image write-platform-metrics",
		"./verity ci patch-image install-otel",
		"./verity ci patch-image emit-platform-span",
		"./verity ci patch-image create-manifest",
		"./verity ci patch-image scan-post",
		"./verity ci patch-image compare-reports",
		"./verity ci patch-image crane copy",
		"./verity ci patch-image resolve-manifest",
		"./verity ci patch-image cosign sign",
		"./verity ci patch-image update-preflight",
		"./verity ci patch-image merge-platform-metrics",
		"./verity ci patch-image build-success-metrics",
		"./verity ci patch-image workflow-start",
		"./verity ci patch-image build-failure-metrics",
	} {
		assert.Contains(t, text, invocation)
	}
	assert.Contains(t, text, "packages: write")
	assert.Contains(t, text, "contents: write")
}
