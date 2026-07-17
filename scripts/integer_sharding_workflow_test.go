package scripts_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegerNightlyWorkflowDispatchesEveryBoundedShard(t *testing.T) {
	// Given
	data, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "integer-orchestrator.yaml"))
	require.NoError(t, err)
	workflow := string(data)

	// Then
	assert.Contains(t, workflow, "shards: ${{ steps.discover.outputs.shards }}")
	assert.Contains(t, workflow, "shard_count: ${{ steps.discover.outputs.shard_count }}")
	assert.Contains(t, workflow, "include: ${{ fromJSON(needs.discover.outputs.shards) }}")
	assert.Contains(t, workflow, "uses: ./.github/workflows/integer-build-shard.yaml")
	assert.Contains(t, workflow, "entries: ${{ matrix.entries }}")
	assert.Contains(t, workflow, "shard: ${{ matrix.shard }}")
	assert.Contains(t, workflow, "CHILD_RESULT: ${{ needs.build-shards.result }}")
	assert.Contains(t, workflow, "BATCH_ID: ${{ github.run_id }}-${{ github.run_attempt }}")
	assert.NotContains(t, workflow, "include: ${{ fromJSON(needs.discover.outputs.images) }}")
}

func TestIntegerShardWorkflowReusesBuildWorkflowAndPropagatesFailures(t *testing.T) {
	// Given
	data, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "integer-build-shard.yaml"))
	require.NoError(t, err)
	workflow := string(data)

	// Then
	assert.Contains(t, workflow, "include: ${{ fromJSON(inputs.entries) }}")
	assert.Contains(t, workflow, "fail-fast: false")
	assert.Contains(t, workflow, "uses: ./.github/workflows/integer-build-image.yaml")
	assert.Contains(t, workflow, "batch_id: ${{ inputs.batch_id }}")
	assert.Contains(t, workflow, "shard: ${{ inputs.shard }}")
	assert.Contains(t, workflow, "secrets: inherit")
}

func TestIntegerNightlyWorkflowAggregatesChildFailuresAndPublishesCurrentReports(t *testing.T) {
	// Given: the nightly parent, shard, and reusable Integer child workflows.
	parentData, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "integer-orchestrator.yaml"))
	require.NoError(t, err)
	shardData, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "integer-build-shard.yaml"))
	require.NoError(t, err)
	childData, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "integer-build-image.yaml"))
	require.NoError(t, err)
	parent := string(parentData)
	shard := string(shardData)
	child := string(childData)

	// Then: every shard and child is awaited, while terminal reports retain correlation.
	assert.Contains(t, parent, "uses: ./.github/workflows/integer-build-shard.yaml")
	assert.Contains(t, parent, "fromJSON(needs.discover.outputs.shards)")
	assert.Contains(t, parent, "fail-fast: false")
	assert.Contains(t, parent, "secrets: inherit")
	assert.Contains(t, parent, "batch_id: ${{ github.run_id }}-${{ github.run_attempt }}")
	assert.Contains(t, parent, `github.event_name }}" = "schedule"`)
	assert.Contains(t, parent, "PLAN_ARGS+=(--force)")
	assert.Contains(t, parent, "CHILD_RESULT: ${{ needs.build-shards.result }}")
	assert.Contains(t, parent, "integer-build-plan-${{ github.run_id }}-${{ github.run_attempt }}")
	assert.Contains(t, parent, "pattern: integer-build-result-*")
	assert.Contains(t, parent, "bash .github/scripts/aggregate-integer-results.sh")

	assert.Contains(t, shard, "fromJSON(inputs.entries)")
	assert.Contains(t, shard, "uses: ./.github/workflows/integer-build-image.yaml")
	assert.Contains(t, shard, "batch_id: ${{ inputs.batch_id }}")
	assert.Contains(t, shard, "shard: ${{ inputs.shard }}")

	assert.Contains(t, child, "workflow_call:")
	assert.Contains(t, child, "batch_id:")
	assert.Contains(t, child, "shard:")
	assert.Contains(t, child, "needs: [melange-prep, melange-build, build]")
	assert.Contains(t, child, "if: always()")
	assert.Contains(t, child, "MELANGE_PREP_RESULT")
	assert.Contains(t, child, "MELANGE_BUILD_RESULT")
	assert.Contains(t, child, "BUILD_RESULT")
	assert.Contains(t, child, "BATCH_ID")
	assert.Contains(t, child, "batch_id: $batch_id")
	assert.Contains(t, child, "shard: $shard")
	assert.Contains(t, child, "integer-build-result-${safe_image}-${INPUT_VERSION}-${INPUT_TYPE}")
	assert.Contains(t, child, "Upload current build report")
}
