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
