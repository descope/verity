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

func TestCLI_testPackage_runsTypedNativePackageCommand(t *testing.T) {
	// Given
	repositoryRoot := t.TempDir()
	called := false
	deps := &cliDependencies{
		commands: commandRunnerFunc(func(_ context.Context, command *Command) (CommandResult, error) {
			called = true
			assert.Equal(t, "melange", command.Name)
			assert.Equal(t, repositoryRoot, command.Dir)
			assert.Contains(t, command.Args, "melange-work/specs/rclone.yaml/build.yaml")
			assert.Contains(t, command.Args, "rclone")
			return CommandResult{}, nil
		}),
		stdout: &bytes.Buffer{},
		getenv: func(string) string { return "" },
	}

	// When
	err := newCLICommand(deps).Run(t.Context(), []string{
		"repository-ops", "test-package", "--kind", "rclone", "--arch", "x86_64", "--repo-root", repositoryRoot,
	})

	// Then
	require.NoError(t, err)
	assert.True(t, called)
}

func TestCLI_parseImageIssue_readsFileAndWritesAllFields(t *testing.T) {
	// Given
	directory := t.TempDir()
	bodyPath := filepath.Join(directory, "issue.md")
	outputPath := filepath.Join(directory, "github-output")
	body := "### Image name\nrclone\n\n### Image repository\nrclone/rclone\n\n### Image tag\nv1.70.3\n\n### Image registry\ndocker.io\n"
	require.NoError(t, os.WriteFile(bodyPath, []byte(body), 0o600))
	var stdout bytes.Buffer
	deps := &cliDependencies{stdout: &stdout, getenv: func(string) string { return "" }}

	// When
	err := newCLICommand(deps).Run(t.Context(), []string{
		"repository-ops", "parse-image-issue", "--body-file", bodyPath, "--github-output", outputPath,
	})

	// Then
	require.NoError(t, err)
	output, readErr := os.ReadFile(outputPath)
	require.NoError(t, readErr)
	assert.Equal(t, "name=rclone\nrepository=rclone/rclone\ntag=v1.70.3\nregistry=docker.io\n", string(output))
	assert.Equal(t, "Parsed: rclone → docker.io/rclone/rclone:v1.70.3\n", stdout.String())
}

func TestCLI_parseImageIssue_requiresFileOrEnvironmentBody(t *testing.T) {
	// Given
	deps := &cliDependencies{stdout: &bytes.Buffer{}, getenv: func(string) string { return "" }}

	// When
	err := newCLICommand(deps).Run(t.Context(), []string{"repository-ops", "parse-image-issue"})

	// Then
	require.ErrorIs(t, err, ErrIssueBodyRequired)
}

func TestCLI_verifySealedSecretsImage_validatesVersionsBeforeRunningDocker(t *testing.T) {
	// Given
	tempDirectory := t.TempDir()
	deps := &cliDependencies{
		commands: commandRunnerFunc(func(context.Context, *Command) (CommandResult, error) {
			t.Fatal("docker must not run for invalid input")
			return CommandResult{}, nil
		}),
		stdout: &bytes.Buffer{},
		getenv: func(name string) string {
			if name == "RUNNER_TEMP" {
				return tempDirectory
			}
			return ""
		},
	}

	// When
	err := newCLICommand(deps).Run(t.Context(), []string{
		"repository-ops", "verify-sealed-secrets-image", "--image", "docker.io/bitnami/sealed-secrets:v1",
		"--version", "!", "--full-version", "v1.0.0",
	})

	// Then
	require.ErrorIs(t, err, ErrNativeVerification)
}
