package workflowpolicy

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateCoherentProducerChain_rejects_semantic_gate_bypasses(t *testing.T) {
	tests := []struct {
		name     string
		mutation workflowMutation
	}{
		{
			name: "chart gate or true",
			mutation: workflowMutation{
				workflow:    "orchestrator.yaml",
				old:         "          needs.discover.outputs.artifact_digest != '' }}",
				replacement: "          needs.discover.outputs.artifact_digest != '' || true }}",
			},
		},
		{
			name: "chart gate false inversion",
			mutation: workflowMutation{
				workflow:    "orchestrator.yaml",
				old:         "          needs.discover.outputs.artifact_digest != '' }}",
				replacement: "          needs.discover.outputs.artifact_digest != '' && false || true }}",
			},
		},
		{
			name: "integration call gate or true",
			mutation: workflowMutation{
				workflow:    "orchestrator.yaml",
				old:         "          needs.chart.outputs.artifact_digest != '' }}",
				replacement: "          needs.chart.outputs.artifact_digest != '' || true }}",
			},
		},
		{
			name: "privileged consumer source gate or true",
			mutation: workflowMutation{
				workflow:    "chart-integration-privileged.yaml",
				old:         "    if: ${{ inputs.source_sha == github.sha }}",
				replacement: "    if: ${{ inputs.source_sha == github.sha || true }}",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given a valid tautological weakening on an actual live consumer gate.
			root := copyLiveCoherentWorkflows(t)
			replaceWorkflowText(t, root, test.mutation)
			workflows, err := loadWorkflows(root)
			require.NoError(t, err)

			// When the coherent policy validates the mutated graph.
			violations := validateCoherentProducerChain(workflows)

			// Then the semantic weakening fails closed.
			require.NotEmpty(t, violations)
			assert.Contains(t, violationRules(violations), RuleProducerIdentity)
		})
	}
}

func TestValidateCoherentProducerChain_rejects_inexact_download_provenance(t *testing.T) {
	tests := []struct {
		name    string
		old     string
		replace string
	}{
		{
			name:    "foreign repository",
			old:     "          repository: ${{ github.repository }}\n",
			replace: "          repository: other-org/other-repo\n",
		},
		{
			name:    "missing run attempt verification",
			old:     "          PROVENANCE_RUN_ATTEMPT: ${{ inputs.run_attempt }}\n",
			replace: "",
		},
		{
			name:    "wrong run attempt verification",
			old:     "          PROVENANCE_RUN_ATTEMPT: ${{ inputs.run_attempt }}\n",
			replace: "          PROVENANCE_RUN_ATTEMPT: '999'\n",
		},
		{
			name:    "wrong digest verification",
			old:     "          PROVENANCE_ARTIFACT_DIGEST: ${{ inputs.artifact_digest }}\n",
			replace: "          PROVENANCE_ARTIFACT_DIGEST: sha256:deadbeef\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given the live chart download redirected or incompletely verified.
			root := copyLiveCoherentWorkflows(t)
			replaceWorkflowText(t, root, workflowMutation{
				workflow:    "chart-gen.yaml",
				old:         test.old,
				replacement: test.replace,
			})
			workflows, err := loadWorkflows(root)
			require.NoError(t, err)

			// When exact download provenance is validated.
			violations := validateCoherentProducerChain(workflows)

			// Then repository, attempt, and digest mismatches fail closed.
			require.NotEmpty(t, violations)
			assert.Contains(t, violationRules(violations), RuleProducerIdentity)
		})
	}
}

func copyLiveCoherentWorkflows(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	sourceRoot := filepath.Join("..", "..", "..", ".github", "workflows")
	for workflowName := range coherentWorkflowInputs {
		data, err := os.ReadFile(filepath.Join(sourceRoot, workflowName))
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(root, workflowName), data, 0o600))
	}
	return root
}
