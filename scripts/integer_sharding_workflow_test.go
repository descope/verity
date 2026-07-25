package scripts_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegerNightlyWorkflowDispatchesEveryBoundedShard(t *testing.T) {
	// Given: the thin nightly wrapper and reusable orchestrator implementation.
	parentData, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "integer-orchestrator.yaml"))
	require.NoError(t, err)
	reusableData, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "integer-orchestrator-reusable.yaml"))
	require.NoError(t, err)
	parent := string(parentData)
	reusable := string(reusableData)

	// Then: the wrapper supplies the canonical Verity artifact to the reusable caller.
	assert.Contains(t, parent, "uses: ./.github/workflows/build-verity.yaml")
	assert.Contains(t, parent, "needs: build-verity")
	assert.Contains(t, parent, "uses: ./.github/workflows/integer-orchestrator-reusable.yaml")
	assert.Contains(t, parent, "event: ${{ github.event_name }}")

	// And: the reusable planner fans out every bounded shard without fail-fast.
	assert.Contains(t, reusable, "shards: ${{ steps.plan-outputs.outputs.shards }}")
	assert.Contains(t, reusable, "shard_count: ${{ steps.plan-outputs.outputs.shard_count }}")
	assert.Contains(t, reusable, "include: ${{ fromJSON(needs.plan.outputs.shards) }}")
	assert.Contains(t, reusable, "fail-fast: false")
	assert.Contains(t, reusable, "uses: ./.github/workflows/integer-build-shard.yaml")
	assert.Contains(t, reusable, "entries: ${{ matrix.entries }}")
	assert.Contains(t, reusable, "component_count: ${{ matrix.component_count }}")
	assert.Contains(t, reusable, "shard: ${{ matrix.shard }}")
	assert.Contains(t, reusable, "plan_artifact_name: ${{ needs.plan.outputs.plan_artifact_name }}")
	assert.Contains(t, reusable, "plan_artifact_digest: ${{ needs.plan.outputs.plan_artifact_digest }}")
	assert.NotContains(t, reusable, "steps.discover.outputs")
}

func TestIntegerShardWorkflowReusesBuildWorkflowAndPropagatesFailures(t *testing.T) {
	// Given: the reusable shard workflow.
	data, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "integer-build-shard.yaml"))
	require.NoError(t, err)
	workflow := string(data)

	// Then: every matrix entry calls the reusable image implementation with exact coordinates.
	assert.Contains(t, workflow, "include: ${{ fromJSON(inputs.entries) }}")
	assert.Contains(t, workflow, "fail-fast: false")
	assert.Contains(t, workflow, "uses: ./.github/workflows/integer-build-image-reusable.yaml")
	assert.Contains(t, workflow, "batch_id: ${{ inputs.batch_id }}")
	assert.Contains(t, workflow, "shard: ${{ inputs.shard }}")
	assert.Contains(t, workflow, "plan_artifact_name: ${{ inputs.plan_artifact_name }}")
	assert.Contains(t, workflow, "plan_artifact_digest: ${{ inputs.plan_artifact_digest }}")
	assert.Contains(t, workflow, "artifact_key: ${{ matrix.artifact_key }}")

	// And: a failed child prevents shard aggregation and cannot be converted to success.
	assert.Contains(t, workflow, "needs: build")
	assert.NotContains(t, workflow, "continue-on-error: true")
	assert.NotContains(t, workflow, "if: always()")
	assert.NotContains(t, workflow, "uses: ./.github/workflows/integer-build-image.yaml")
}

func TestIntegerNightlyWorkflowAggregatesChildFailuresAndPublishesExactManifest(t *testing.T) {
	// Given: the thin parent, reusable orchestrator, shard, and image workflows.
	parentData, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "integer-orchestrator.yaml"))
	require.NoError(t, err)
	orchestratorData, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "integer-orchestrator-reusable.yaml"))
	require.NoError(t, err)
	shardData, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "integer-build-shard.yaml"))
	require.NoError(t, err)
	childData, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "integer-build-image-reusable.yaml"))
	require.NoError(t, err)
	parent := string(parentData)
	orchestrator := string(orchestratorData)
	shard := string(shardData)
	child := string(childData)

	// Then: the parent delegates execution and never owns a local aggregator.
	assert.Contains(t, parent, "uses: ./.github/workflows/build-verity.yaml")
	assert.Contains(t, parent, "uses: ./.github/workflows/integer-orchestrator-reusable.yaml")
	assert.Contains(t, parent, "verity_artifact_digest: ${{ needs.build-verity.outputs.artifact-digest }}")
	assert.NotContains(t, parent, "aggregate-integer-results.sh")

	// And: the reusable orchestrator propagates child failure before typed aggregation.
	assert.Contains(t, orchestrator, "needs: [plan, build-shards]")
	assert.Contains(t, orchestrator, "always() && needs.plan.result == 'success'")
	assert.Contains(t, orchestrator, "needs.build-shards.result == 'success'")
	assert.Contains(t, orchestrator, "needs.plan.outputs.count == '0' && needs.build-shards.result == 'skipped'")
	assert.Contains(t, orchestrator, "./verity ci integer-batch aggregate")
	assert.Contains(t, orchestrator, "--plan integer-plan/plan.json")
	assert.Contains(t, orchestrator, "--shards-dir integer-shards")
	assert.Contains(t, orchestrator, "--output integer-manifest.json")
	assert.Contains(t, orchestrator, "Download exact shard manifests")
	assert.Contains(t, orchestrator, "pattern: integer-shard-manifest-${{ needs.plan.outputs.publication_id }}-*")
	assert.Contains(t, orchestrator, "name: integer-manifest-${{ needs.plan.outputs.publication_id }}")
	assert.NotContains(t, orchestrator, "aggregate-integer-results.sh")

	// And: shard and image calls retain exact plan, Verity, and component coordinates.
	assert.Contains(t, shard, "uses: ./.github/workflows/integer-build-image-reusable.yaml")
	assert.Contains(t, shard, "plan_artifact_name: ${{ inputs.plan_artifact_name }}")
	assert.Contains(t, shard, "plan_artifact_digest: ${{ inputs.plan_artifact_digest }}")
	assert.Contains(t, shard, "artifact_key: ${{ matrix.artifact_key }}")
	assert.Contains(t, child, "uses: ./.github/actions/setup-verity")
	assert.Contains(t, child, "artifact-name: ${{ inputs.verity_artifact_name }}")
	assert.Contains(t, child, "artifact-digest: ${{ inputs.verity_artifact_digest }}")
	assert.Contains(t, child, "build-key: ${{ inputs.verity_build_key }}")
	assert.Contains(t, child, "name: integer-component-${{ inputs.publication_id }}-${{ inputs.shard }}-${{ inputs.artifact_key }}")
	assert.Contains(t, child, "name: Upload exact Integer component")
}
