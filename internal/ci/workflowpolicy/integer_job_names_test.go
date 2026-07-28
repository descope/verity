package workflowpolicy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestIntegerBuildImageReusable_jobsNameTargetAndStage(t *testing.T) {
	// Given: the production reusable workflow expanded once per Integer target.
	var parsed struct {
		Jobs map[string]struct {
			Name string `yaml:"name"`
		} `yaml:"jobs"`
	}
	require.NoError(t, yaml.Unmarshal(
		[]byte(readRepositoryIntegerWorkflowText(t, "integer-build-image-reusable.yaml")),
		&parsed,
	))

	// When: GitHub renders each reusable job's display name.
	want := map[string]string{
		"validate-coordinates": "Validate ${{ inputs.image }}:${{ inputs.version }}-${{ inputs.type }} coordinates",
		"melange-prep":         "Prepare ${{ inputs.image }}:${{ inputs.version }}-${{ inputs.type }} recipe",
		"melange-build":        "Build ${{ inputs.image }}:${{ inputs.version }}-${{ inputs.type }} APKs (${{ matrix.arch }})",
		"build":                "Publish ${{ inputs.image }}:${{ inputs.version }}-${{ inputs.type }}",
	}

	// Then: every repeated job identifies both its target and its current stage.
	for job, name := range want {
		require.Contains(t, parsed.Jobs, job)
		assert.Equal(t, name, parsed.Jobs[job].Name, "job %s", job)
	}
}
