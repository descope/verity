package repositoryops

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCLI_scanBefore_writesTypedCountsAndSummary(t *testing.T) {
	// Given
	reportPath := filepath.Join(t.TempDir(), "before.json")
	environmentPath := filepath.Join(t.TempDir(), "github-env")
	var stdout bytes.Buffer
	deps := &cliDependencies{
		commands: commandRunnerFunc(func(_ context.Context, command *Command) (CommandResult, error) {
			assert.Equal(t, reportPath, commandFlagValue(command.Args, "--output"))
			report := `{"Results":[{"Type":"gobinary","Vulnerabilities":[{},{}]},{"Type":"alpine","Vulnerabilities":[{}]}]}`
			require.NoError(t, os.WriteFile(reportPath, []byte(report), 0o600))
			return CommandResult{}, nil
		}),
		stdout: &stdout,
		getenv: func(string) string { return "" },
	}

	// When
	err := newCLICommand(deps).Run(t.Context(), []string{
		"repository-ops", "scan-before", "--source", "docker.io/library/nginx:v1",
		"--report", reportPath, "--github-env", environmentPath,
	})

	// Then
	require.NoError(t, err)
	output, readErr := os.ReadFile(environmentPath)
	require.NoError(t, readErr)
	assert.Equal(t, "before_total=3\nbefore_go=2\nbefore_non_go=1\n", string(output))
	assert.Equal(t, "BEFORE — total: 3, non-go: 1, go: 2\n", stdout.String())
}

func TestCLI_verifyPatched_writesBeforeAfterSummary(t *testing.T) {
	// Given
	reportPath := filepath.Join(t.TempDir(), "after.json")
	var stdout bytes.Buffer
	deps := &cliDependencies{
		commands: commandRunnerFunc(func(_ context.Context, command *Command) (CommandResult, error) {
			assert.Equal(t, reportPath, commandFlagValue(command.Args, "--output"))
			report := `{"Results":[{"Type":"gobinary","Vulnerabilities":[{}]}]}`
			require.NoError(t, os.WriteFile(reportPath, []byte(report), 0o600))
			return CommandResult{}, nil
		}),
		stdout: &stdout,
		getenv: func(string) string { return "" },
	}

	// When
	err := newCLICommand(deps).Run(t.Context(), []string{
		"repository-ops", "verify-patched", "--image", "ghcr.io/verity/nginx:v1", "--image-label", "nginx/amd64",
		"--report", reportPath, "--before-total", "3", "--before-go", "2", "--before-non-go", "1",
	})

	// Then
	require.NoError(t, err)
	assert.Equal(t, "nginx/amd64 — BEFORE 3 (1 non-Go, 2 Go); AFTER 1 (0 non-Go, 1 Go)\n", stdout.String())
}

func TestCLI_catalogEntry_writesSourceAndGoVCSOutputs(t *testing.T) {
	// Given
	directory := t.TempDir()
	configPath := filepath.Join(directory, "copa-config.yaml")
	outputPath := filepath.Join(directory, "github-output")
	config := `images:
  - name: controller
    image: quay.io/example/controller
    goVcsUrl: https://github.com/example/controller
    goVcsTagPrefix: v
    tags:
      strategy: list
      list: [1.2.3]
`
	require.NoError(t, os.WriteFile(configPath, []byte(config), 0o600))
	var stdout bytes.Buffer
	deps := &cliDependencies{stdout: &stdout, getenv: func(string) string { return "" }}

	// When
	err := newCLICommand(deps).Run(t.Context(), []string{
		"repository-ops", "catalog-entry", "--config", configPath, "--image-name", "controller",
		"--image-tag", "1.2.3", "--github-output", outputPath,
	})

	// Then
	require.NoError(t, err)
	output, readErr := os.ReadFile(outputPath)
	require.NoError(t, readErr)
	assert.Equal(t, "source=quay.io/example/controller:1.2.3\ngo_vcs_url=https://github.com/example/controller@v1.2.3\n", string(output))
	assert.Equal(t, "Catalog: image=quay.io/example/controller:1.2.3 goVcsUrl=https://github.com/example/controller@v1.2.3\n", stdout.String())
}
