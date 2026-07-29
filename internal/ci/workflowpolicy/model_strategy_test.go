package workflowpolicy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestWorkflowStrategy_UnmarshalYAML_tracksPresence(t *testing.T) {
	var strategy workflowStrategy

	err := yaml.Unmarshal([]byte("matrix:\n  arch: [amd64]\n"), &strategy)

	require.NoError(t, err)
	assert.True(t, strategy.Present)
	assert.Equal(t, yaml.MappingNode, strategy.Matrix.Kind)
}
