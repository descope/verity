package scripts_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPRWorkflowProvidesBaseDefinitionsToIntegerPlanner(t *testing.T) {
	// Given: the pull-request workflow that invokes semantic Integer planning.
	data, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "pr-test.yaml"))
	require.NoError(t, err)
	workflow := string(data)

	// Then: the typed planner owns base-definition materialization.
	assert.Contains(t, workflow, "./verity ci pr-test plan-integer")
	assert.Contains(t, workflow, `--base-sha "$BASE_SHA"`)
	assert.Contains(t, workflow, `--head-sha "$HEAD_SHA"`)
	assert.Contains(t, workflow, `--temp-dir "$RUNNER_TEMP"`)
}
