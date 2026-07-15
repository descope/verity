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

	// Then: changed base definitions are materialized and passed to the planner.
	assert.Contains(t, workflow, `git show "${BASE_SHA}:${file}" > "$RUNNER_TEMP/base-images/$relative"`)
	assert.Contains(t, workflow, `--base-images-dir "$RUNNER_TEMP/base-images"`)
}
