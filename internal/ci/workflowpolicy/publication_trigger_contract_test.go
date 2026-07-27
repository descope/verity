package workflowpolicy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPublishWorkflow_triggersOnlyRelevantProtectedChanges(t *testing.T) {
	// Given: the production publication workflow.
	workflow := readRepositoryIntegerWorkflow(t, "publish.yaml")
	workflowText := readRepositoryIntegerWorkflowText(t, "publish.yaml")

	// When: its automatic trigger contract is inspected.
	paths := workflow.On.Push.Paths

	// Then: protected main, nightly publication, and every producer input class are covered.
	require.True(t, workflow.On.Push.Present)
	require.Contains(t, workflowText, "branches: [main]")
	require.True(t, workflow.On.Schedule)
	for _, required := range []string{
		"integer.yaml", "images/**", "packages/bespoke/**", "packages/pipelines/**",
		"packages/overrides/**", "packages/upstream.lock.json", "copa-config.yaml", "Chart.yaml",
		"verity.yaml", "site/**", "keys/apk/**", "ci/apk-signer.lock.json",
		"ci/apk-signing-key-state.json", "*.go", "**/*.go", "go.mod", "go.sum",
		"mise.toml", "mise.lock", ".github/actions/setup-verity/**", ".github/workflows/publish.yaml",
		".github/workflows/build-site.yaml", ".github/workflows/build-verity.yaml",
		".github/workflows/build-verity-protected.yaml", ".github/workflows/orchestrator.yaml",
		".github/workflows/patch-image.yaml", ".github/workflows/integer-orchestrator-reusable.yaml",
		".github/workflows/integer-build-shard.yaml", ".github/workflows/integer-build-image.yaml",
		".github/workflows/integer-build-image-reusable.yaml",
	} {
		assert.Contains(t, paths, required)
	}
	assert.NotContains(t, paths, "**")
}

func TestBuildSite_passesTypedBooleanPreflightInput(t *testing.T) {
	// Given: the exact Build Site reusable caller text.
	workflow := readRepositoryIntegerWorkflowText(t, "build-site.yaml")

	// When: the chart producer's boolean input is inspected.

	// Then: GitHub receives a boolean scalar rather than a rejected string.
	require.Contains(t, workflow, "      preflight: false\n")
	require.NotContains(t, workflow, "      preflight: \"false\"\n")
}
