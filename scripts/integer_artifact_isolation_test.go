package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestIntegerBuildWorkflowIsolatesMelangeArtifactsPerEntry(t *testing.T) {
	// Given
	data, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "integer-build-image.yaml"))
	require.NoError(t, err)
	var workflow struct {
		Jobs map[string]struct {
			Steps []struct {
				ID   string            `yaml:"id"`
				Uses string            `yaml:"uses"`
				Run  string            `yaml:"run"`
				With map[string]string `yaml:"with"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	require.NoError(t, yaml.Unmarshal(data, &workflow))

	// When
	var keyCommand string
	var intermediateNames []string
	for _, job := range workflow.Jobs {
		for _, step := range job.Steps {
			if step.ID == "artifact-key" {
				keyCommand = step.Run
			}
			name := step.With["name"]
			if strings.Contains(step.Uses, "artifact") &&
				(strings.Contains(name, "melange-work") || strings.Contains(name, "melange-packages")) {
				intermediateNames = append(intermediateNames, name)
			}
		}
	}

	// Then
	require.NotEmpty(t, keyCommand)
	require.Len(t, intermediateNames, 6)
	for _, name := range intermediateNames {
		assert.Contains(t, name, "artifact_key", name)
	}

	first := runArtifactKeyCommand(t, keyCommand, "team/a-b", "1", "default")
	repeat := runArtifactKeyCommand(t, keyCommand, "team/a-b", "1", "default")
	collisionCandidate := runArtifactKeyCommand(t, keyCommand, "team-a/b", "1", "default")
	assert.Equal(t, first, repeat)
	assert.NotEqual(t, first, collisionCandidate)
	assert.Regexp(t, regexp.MustCompile(`^[A-Za-z0-9._-]+-[0-9a-f]{12}$`), first)
}

func runArtifactKeyCommand(t *testing.T, command, image, version, imageType string) string {
	t.Helper()
	outputPath := filepath.Join(t.TempDir(), "github-output")
	cmd := exec.CommandContext(t.Context(), "bash", "-c", command)
	cmd.Env = append(
		os.Environ(),
		"INPUT_IMAGE="+image,
		"INPUT_VERSION="+version,
		"INPUT_TYPE="+imageType,
		"GITHUB_OUTPUT="+outputPath,
	)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, string(output))
	data, err := os.ReadFile(outputPath)
	require.NoError(t, err)
	return strings.TrimPrefix(strings.TrimSpace(string(data)), "artifact_key=")
}

func TestIntegerBuildWorkflowRetriesEveryVerityBuild(t *testing.T) {
	// Given
	data, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "integer-build-image.yaml"))
	require.NoError(t, err)
	workflow := string(data)

	// Then
	assert.Equal(t, 3, strings.Count(workflow, "bash .github/scripts/retry-go-build.sh"))
	assert.NotContains(t, workflow, "run: go build -o verity .")
}

func TestRetryGoBuildRetriesTransientModuleDownloadFailure(t *testing.T) {
	// Given
	tmp := t.TempDir()
	binDir := filepath.Join(tmp, "bin")
	attemptPath := filepath.Join(tmp, "attempts")
	writeExecutable(t, filepath.Join(binDir, "go"), `#!/usr/bin/env bash
set -euo pipefail
attempt=0
if [ -f "$GO_ATTEMPT_FILE" ]; then
  attempt=$(cat "$GO_ATTEMPT_FILE")
fi
attempt=$((attempt + 1))
printf '%s\n' "$attempt" > "$GO_ATTEMPT_FILE"
if [ "$attempt" -eq 1 ]; then
  echo 'transient proxy failure' >&2
  exit 1
fi
test "$*" = 'build -o verity .'
touch verity
`)
	helper, err := filepath.Abs(filepath.Join("..", ".github", "scripts", "retry-go-build.sh"))
	require.NoError(t, err)

	// When
	cmd := exec.CommandContext(t.Context(), "bash", helper)
	cmd.Dir = tmp
	cmd.Env = append(
		os.Environ(),
		"PATH="+binDir+":"+os.Getenv("PATH"),
		"GO_ATTEMPT_FILE="+attemptPath,
		"REGISTRY_COMMAND_ATTEMPTS=2",
		"REGISTRY_COMMAND_BASE_DELAY_SECONDS=1",
	)
	output, err := cmd.CombinedOutput()

	// Then
	require.NoError(t, err, string(output))
	assert.FileExists(t, filepath.Join(tmp, "verity"))
	attempts, err := os.ReadFile(attemptPath)
	require.NoError(t, err)
	assert.Equal(t, "2", strings.TrimSpace(string(attempts)))
	assert.Contains(t, string(output), "attempt 2/2")
}
