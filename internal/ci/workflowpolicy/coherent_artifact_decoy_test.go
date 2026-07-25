package workflowpolicy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateCoherentProducerChain_accepts_exact_fixture(t *testing.T) {
	// Given the canonical exact producer fixture.
	root := copyCoherentWorkflowFixture(t)
	workflows, err := loadWorkflows(root)
	require.NoError(t, err)

	// When the coherent producer policy validates it.
	violations := validateCoherentProducerChain(workflows)

	// Then the complete exact graph is accepted.
	assert.Empty(t, violations)
}

func TestValidateCoherentProducerChain_rejects_untracked_artifact_consumer_decoy(t *testing.T) {
	// Given an exact download job that is unrelated to the integration result graph.
	root := copyCoherentWorkflowFixture(t)
	replaceWorkflowText(t, root, workflowMutation{
		workflow: "chart-integration.yaml",
		old:      "  result:\n",
		replacement: "  untracked-download:\n" +
			"    if: ${{ inputs.source_sha == github.sha }}\n" +
			"    runs-on: ubuntu-24.04\n" +
			"    permissions:\n" +
			"      actions: read\n" +
			"    steps:\n" +
			"      - uses: actions/download-artifact@3e5f45b2cfb9172054b4087a40e8e0b5a5461e7c\n" +
			"        with:\n" +
			"          name: ${{ inputs.artifact_name }}\n" +
			"          run-id: ${{ inputs.run_id }}\n" +
			"          github-token: ${{ secrets.GITHUB_TOKEN }}\n\n" +
			"  result:\n",
	})
	workflows, err := loadWorkflows(root)
	require.NoError(t, err)

	// When the exact integration graph is validated.
	violations := validateCoherentProducerChain(workflows)

	// Then an unrelated artifact consumer cannot satisfy or bypass the gate.
	require.NotEmpty(t, violations)
	assert.Contains(t, violationRules(violations), RuleProducerIdentity)
}

func TestValidateCoherentProducerChain_rejects_decoy_replacing_real_artifact_consumer(t *testing.T) {
	// Given the real test job no longer downloads the artifact while a tracked decoy does.
	root := copyCoherentWorkflowFixture(t)
	for _, mutation := range []workflowMutation{
		{
			workflow:    "chart-integration.yaml",
			old:         "        uses: actions/download-artifact@3e5f45b2cfb9172054b4087a40e8e0b5a5461e7c\n",
			replacement: "        uses: actions/checkout@9c091bb21b7c1c1d1991bb908d89e4e9dddfe3e0\n",
		},
		{
			workflow: "chart-integration.yaml",
			old:      "  result:\n",
			replacement: "  decoy-download:\n" +
				"    if: ${{ inputs.source_sha == github.sha }}\n" +
				"    runs-on: ubuntu-24.04\n" +
				"    permissions:\n" +
				"      actions: read\n" +
				"    steps:\n" +
				"      - uses: actions/download-artifact@3e5f45b2cfb9172054b4087a40e8e0b5a5461e7c\n" +
				"        with:\n" +
				"          name: ${{ inputs.artifact_name }}\n" +
				"          run-id: ${{ inputs.run_id }}\n" +
				"          github-token: ${{ secrets.GITHUB_TOKEN }}\n\n" +
				"  result:\n",
		},
		{
			workflow: "chart-integration.yaml",
			old: "    needs: [discover-charts, chart-test]\n" +
				"    if: ${{ needs.discover-charts.result == 'success' && (needs.chart-test.result == 'success' || needs.chart-test.result == 'skipped') }}\n",
			replacement: "    needs: [discover-charts, chart-test, decoy-download]\n" +
				"    if: ${{ needs.discover-charts.result == 'success' && (needs.chart-test.result == 'success' || needs.chart-test.result == 'skipped') && needs.decoy-download.result == 'success' }}\n",
		},
	} {
		replaceWorkflowText(t, root, mutation)
	}
	workflows, err := loadWorkflows(root)
	require.NoError(t, err)

	// When the integration graph is validated.
	violations := validateCoherentProducerChain(workflows)

	// Then the real consumer's missing provenance cannot be hidden by the decoy.
	require.NotEmpty(t, violations)
	assert.Contains(t, violationRules(violations), RuleProducerIdentity)
}

func TestValidateCoherentProducerChain_rejects_batch_id_as_artifact_identity(t *testing.T) {
	// Given a chart artifact consistently renamed with batch_id instead of publication_id.
	root := copyCoherentWorkflowFixture(t)
	for _, mutation := range []workflowMutation{
		{
			workflow:    "chart-gen.yaml",
			old:         "      artifact_name: chart-publication-${{ inputs.publication_id }}\n",
			replacement: "      artifact_name: chart-publication-${{ inputs.batch_id }}\n",
		},
		{
			workflow:    "chart-gen.yaml",
			old:         "          name: chart-publication-${{ inputs.publication_id }}\n",
			replacement: "          name: chart-publication-${{ inputs.batch_id }}\n",
		},
	} {
		replaceWorkflowText(t, root, mutation)
	}
	workflows, err := loadWorkflows(root)
	require.NoError(t, err)

	// When the immutable artifact contract is validated.
	violations := validateCoherentProducerChain(workflows)

	// Then batch identity cannot substitute for publication identity.
	require.NotEmpty(t, violations)
	assert.Contains(t, violationRules(violations), RuleProducerIdentity)
}
