package workflowpolicy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateDirectory_rejects_uncorrelated_needs_output_producers(t *testing.T) {
	tests := []struct {
		name        string
		old         string
		replacement string
	}{
		{
			name:        "unrelated run producer",
			old:         "          run-id: ${{ inputs.run_id }}",
			replacement: "          run-id: ${{ inputs.attacker_run_id }}",
		},
		{
			name:        "mixed artifact producer",
			old:         "          name: ${{ inputs.artifact_name }}",
			replacement: "          name: ${{ inputs.attacker_artifact_name }}",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given a reusable artifact selector whose identity comes from an unrelated input.
			root := copyCoherentWorkflowFixture(t)
			replaceWorkflowText(t, root, workflowMutation{
				workflow: "chart-gen.yaml", old: test.old, replacement: test.replacement,
			})

			// When workflow policy validates the producer relationship.
			_, err := ValidateDirectory(root)

			// Then field-name-only producer spoofing fails closed.
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrPolicyViolation)
			assert.ErrorContains(t, err, string(RuleProducerIdentity))
		})
	}
}

func TestValidateDirectory_rejects_static_run_identity_fallback(t *testing.T) {
	// Given a numeric run ID detached from the reusable producer input.
	root := copyCoherentWorkflowFixture(t)
	replaceWorkflowText(t, root, workflowMutation{
		workflow: "chart-gen.yaml", old: "          run-id: ${{ inputs.run_id }}", replacement: `          run-id: "12345"`,
	})

	// When workflow policy validates the reusable cross-run download.
	_, err := ValidateDirectory(root)

	// Then an immutable-looking static fallback is not trusted.
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPolicyViolation)
	assert.ErrorContains(t, err, string(RuleProducerIdentity))
}

func TestValidateDirectory_rejects_cross_run_download_without_explicit_producer_graph(t *testing.T) {
	// Given the chart producer is omitted from the current orchestrator needs graph.
	root := copyCoherentWorkflowFixture(t)
	replaceWorkflowText(t, root, workflowMutation{
		workflow: "orchestrator.yaml", old: "  chart:\n    needs: [discover, patch]", replacement: "  chart:\n    needs: discover",
	})

	// When workflow policy validates the producer graph.
	_, err := ValidateDirectory(root)

	// Then an implicit/nonexistent producer cannot authorize artifact selection.
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPolicyViolation)
	assert.ErrorContains(t, err, string(RuleProducerIdentity))
}

func TestValidateDirectory_rejects_incomplete_or_untrusted_producer_contracts(t *testing.T) {
	tests := []struct {
		name     string
		mutation workflowMutation
	}{
		{
			name: "untrusted top-level reusable workflow",
			mutation: workflowMutation{
				workflow:    "orchestrator.yaml",
				old:         "    uses: ./.github/workflows/chart-gen.yaml",
				replacement: "    uses: ./.github/workflows/patch-image.yaml",
			},
		},
		{
			name: "producer runs after failure",
			mutation: workflowMutation{
				workflow: "orchestrator.yaml", old: "      ${{ needs.discover.result == 'success' &&\n",
				replacement: "      ${{ always() &&\n",
			},
		},
		{
			name: "consumer runs after failure",
			mutation: workflowMutation{
				workflow: "orchestrator.yaml", old: "      ${{ needs.chart.result == 'success' &&\n",
				replacement: "      ${{ always() &&\n",
			},
		},
		{
			name: "required artifact name output missing",
			mutation: workflowMutation{
				workflow: "chart-gen.yaml",
				old:      "      artifact_name:\n        value: ${{ jobs.generate.outputs.artifact_name }}\n",
			},
		},
		{
			name: "outputs split across jobs",
			mutation: workflowMutation{
				workflow:    "chart-gen.yaml",
				old:         "${{ jobs.generate.outputs.artifact_digest }}",
				replacement: "${{ jobs.attacker.outputs.artifact_digest }}",
			},
		},
		{
			name: "terminal artifact name output static",
			mutation: workflowMutation{
				workflow: "chart-gen.yaml", old: "      artifact_name: chart-publication-${{ inputs.publication_id }}",
				replacement: "      artifact_name: chart-publication-static",
			},
		},
		{
			name: "artifact digest detached from upload",
			mutation: workflowMutation{
				workflow:    "chart-gen.yaml",
				old:         "      artifact_digest: ${{ steps.upload.outputs.artifact-digest }}",
				replacement: "      artifact_digest: ${{ github.sha }}",
			},
		},
		{
			name: "artifact name static prefix fallback",
			mutation: workflowMutation{
				workflow: "chart-gen.yaml", old: "          name: chart-publication-${{ inputs.publication_id }}",
				replacement: "          name: fallback-${{ inputs.publication_id }}",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given one current reusable producer graph or output-contract trust edge is weakened.
			root := copyCoherentWorkflowFixture(t)
			replaceWorkflowText(t, root, test.mutation)

			// When workflow policy validates the complete producer chain.
			_, err := ValidateDirectory(root)

			// Then incomplete, untrusted, or failure-tolerant producers are rejected.
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrPolicyViolation)
			assert.ErrorContains(t, err, string(RuleProducerIdentity))
		})
	}
}
