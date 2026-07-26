package repositoryops

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errCLIPatchFailure = errors.New("sentinel patch failure")

func TestCLI_patchImage_writesDigestAndSuccessfulExitCode(t *testing.T) {
	// Given
	outputPath := filepath.Join(t.TempDir(), "github-output")
	digest := "sha256:" + strings.Repeat("a", 64)
	var stdout bytes.Buffer
	var patched PatchSpec
	commandCalls := 0
	deps := &cliDependencies{
		patcher: patcherFunc(func(_ context.Context, spec *PatchSpec) error {
			patched = *spec
			return nil
		}),
		commands: commandRunnerFunc(func(_ context.Context, command *Command) (CommandResult, error) {
			commandCalls++
			assert.Equal(t, "crane", command.Name)
			return CommandResult{Stdout: []byte(digest + "\n")}, nil
		}),
		stdout: &stdout,
		getenv: func(string) string { return "" },
	}

	// When
	err := newCLICommand(deps).Run(t.Context(), []string{
		"repository-ops", "patch-image", "--platform", "linux/amd64", "--source", "docker.io/library/nginx:v1",
		"--image-name", "nginx", "--staging-registry", "ghcr.io/verity/cache", "--github-output", outputPath,
	})

	// Then
	require.NoError(t, err)
	assert.Equal(t, 2, commandCalls)
	assert.Equal(t, "docker.io/library/nginx:v1", patched.Source)
	assert.Equal(t, "ghcr.io/verity/cache:nginx-v1-amd64", patched.Destination)
	output, readErr := os.ReadFile(outputPath)
	require.NoError(t, readErr)
	assert.Contains(t, string(output), "exit-code=0\n")
	assert.Contains(t, string(output), "staging-digest="+digest+"\n")
	assert.Contains(t, stdout.String(), "Patched platform-specific image: ghcr.io/verity/cache:nginx-v1-amd64")
}

func TestCLI_patchImage_writesFailureExitCodeWhenPatcherFails(t *testing.T) {
	// Given
	outputPath := filepath.Join(t.TempDir(), "github-output")
	deps := &cliDependencies{
		patcher: patcherFunc(func(context.Context, *PatchSpec) error { return errCLIPatchFailure }),
		commands: commandRunnerFunc(func(context.Context, *Command) (CommandResult, error) {
			t.Fatal("command runner must not be called after patch failure")
			return CommandResult{}, nil
		}),
		stdout: &bytes.Buffer{},
		getenv: func(string) string { return "" },
	}

	// When
	err := newCLICommand(deps).Run(t.Context(), []string{
		"repository-ops", "patch-image", "--platform", "linux/amd64", "--source", "docker.io/library/nginx:v1",
		"--image-name", "nginx", "--staging-registry", "ghcr.io/verity/cache", "--github-output", outputPath,
	})

	// Then
	require.ErrorIs(t, err, errCLIPatchFailure)
	output, readErr := os.ReadFile(outputPath)
	require.NoError(t, readErr)
	assert.Contains(t, string(output), "exit-code=1\n")
	assert.NotContains(t, string(output), "staging-digest=")
}
