package scripts_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestIntegerBuildWorkflowIsolatesMelangeArtifactsPerEntry(t *testing.T) {
	// Given
	data, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "integer-build-image-reusable.yaml"))
	require.NoError(t, err)
	var workflow struct {
		Jobs map[string]struct {
			Steps []struct {
				Uses string            `yaml:"uses"`
				With map[string]string `yaml:"with"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	require.NoError(t, yaml.Unmarshal(data, &workflow))

	// When
	var intermediateNames []string
	var componentNames []string
	for _, job := range workflow.Jobs {
		for _, step := range job.Steps {
			name := step.With["name"]
			if strings.Contains(step.Uses, "actions/upload-artifact") &&
				(strings.Contains(name, "melange-work") || strings.Contains(name, "melange-packages")) {
				intermediateNames = append(intermediateNames, name)
			}
			if strings.Contains(step.Uses, "actions/upload-artifact") && strings.HasPrefix(name, "integer-component-") {
				componentNames = append(componentNames, name)
			}
		}
	}

	// Then
	require.Len(t, intermediateNames, 2)
	nameCounts := map[string]int{}
	for _, name := range intermediateNames {
		nameCounts[name]++
	}
	assert.Equal(t, 1, nameCounts[`${{ inputs.publication_id }}-${{ inputs.artifact_key }}-melange-work`])
	assert.Equal(t, 1, nameCounts[`${{ needs.melange-prep.outputs.publication_id }}-${{ needs.melange-prep.outputs.artifact_key }}-melange-packages-${{ matrix.arch }}`])
	require.Equal(t, []string{
		`integer-component-${{ inputs.publication_id }}-${{ inputs.shard }}-${{ inputs.artifact_key }}`,
	}, componentNames)
	for _, name := range append(intermediateNames, componentNames...) {
		assert.Contains(t, name, "artifact_key", name)
	}
}

func TestIntegerBuildWorkflowUsesCanonicalVerityArtifact(t *testing.T) {
	// Given
	data, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "integer-build-image.yaml"))
	require.NoError(t, err)
	wrapper := string(data)
	reusableData, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "integer-build-image-reusable.yaml"))
	require.NoError(t, err)
	reusable := string(reusableData)

	// Then
	assert.Equal(t, 1, strings.Count(wrapper, "uses: ./.github/workflows/build-verity-protected.yaml"))
	assert.Contains(t, wrapper, "needs: build-verity")
	assert.Contains(t, wrapper, "uses: ./.github/workflows/integer-build-image-reusable.yaml")
	assert.Contains(t, reusable, "uses: ./.github/actions/setup-verity")
	assert.Contains(t, reusable, "artifact-name: ${{ inputs.verity_artifact_name }}")
	assert.Contains(t, reusable, "artifact-digest: ${{ inputs.verity_artifact_digest }}")
	assert.Contains(t, reusable, "build-key: ${{ inputs.verity_build_key }}")
	for _, workflow := range []string{wrapper, reusable} {
		assert.NotContains(t, workflow, "go build -o verity .")
		assert.NotContains(t, workflow, "retry-go-build.sh")
	}
}
