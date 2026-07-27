package workflowpolicy

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateCoherentProducerChain_rejects_reviewed_bypasses(t *testing.T) {
	tests := []struct {
		name      string
		mutations []workflowMutation
	}{
		{
			name: "chart consumer weakened to not cancelled",
			mutations: []workflowMutation{{
				workflow:    "orchestrator.yaml",
				old:         "          needs.discover.outputs.artifact_digest != '' }}",
				replacement: "          needs.discover.outputs.artifact_digest != '' || !cancelled() }}",
			}},
		},
		{
			name: "decoy source gate hides unguarded integration consumer",
			mutations: []workflowMutation{
				{
					workflow:    "chart-integration.yaml",
					old:         "    if: ${{ github.event_name == 'pull_request' || inputs.source_sha == github.sha }}\n",
					replacement: "    if: ${{ github.event_name == 'pull_request' }}\n",
				},
				{
					workflow: "chart-integration.yaml",
					old:      "  result:\n",
					replacement: "  decoy-source-gate:\n" +
						"    if: ${{ inputs.source_sha == github.sha }}\n" +
						"    runs-on: ubuntu-24.04\n" +
						"    permissions: {}\n" +
						"    steps:\n" +
						"      - run: echo decoy\n\n" +
						"  result:\n",
				},
			},
		},
		{
			name: "wrong same run artifact download",
			mutations: []workflowMutation{{
				workflow: "chart-gen.yaml",
				old:      "    steps:\n      - uses: actions/download-artifact@",
				replacement: "    steps:\n" +
					"      - uses: actions/download-artifact@3e5f45b2cfb9172054b4087a40e8e0b5a5461e7c\n" +
					"        with:\n" +
					"          name: wrong-producer-artifact\n" +
					"      - uses: actions/download-artifact@",
			}},
		},
		{
			name: "wrong same run artifact hidden by exact decoy download",
			mutations: []workflowMutation{
				{
					workflow:    "chart-gen.yaml",
					old:         "          name: ${{ inputs.artifact_name }}\n",
					replacement: "          name: wrong-same-run-artifact\n",
				},
				{
					workflow: "chart-gen.yaml",
					old:      "jobs:\n  generate:\n",
					replacement: "jobs:\n" +
						"  decoy-download:\n" +
						"    runs-on: ubuntu-24.04\n" +
						"    permissions:\n" +
						"      actions: read\n" +
						"    steps:\n" +
						"      - uses: actions/download-artifact@3e5f45b2cfb9172054b4087a40e8e0b5a5461e7c\n" +
						"        with:\n" +
						"          name: ${{ inputs.artifact_name }}\n" +
						"          run-id: ${{ inputs.run_id }}\n" +
						"          github-token: ${{ secrets.GITHUB_TOKEN }}\n\n" +
						"  generate:\n",
				},
			},
		},
		{
			name: "unrelated run identity download",
			mutations: []workflowMutation{{
				workflow:    "chart-gen.yaml",
				old:         "          run-id: ${{ inputs.run_id }}\n",
				replacement: "          run-id: ${{ github.run_id }}\n",
			}},
		},
		{
			name: "artifact digest comes from unrelated producer",
			mutations: []workflowMutation{{
				workflow:    "orchestrator.yaml",
				old:         "      artifact_digest: ${{ needs.chart.outputs.artifact_digest }}\n",
				replacement: "      artifact_digest: ${{ needs.discover.outputs.artifact_digest }}\n",
			}},
		},
		{
			name: "expected artifact digest made optional",
			mutations: []workflowMutation{{
				workflow: "chart-gen.yaml",
				old:      "      artifact_digest:\n        required: true",
				replacement: "      artifact_digest:\n" +
					"        required: false",
			}},
		},
		{
			name: "publication identity substituted with batch identity",
			mutations: []workflowMutation{{
				workflow:    "orchestrator.yaml",
				old:         "      publication_id: ${{ needs.discover.outputs.publication_id }}\n",
				replacement: "      publication_id: ${{ needs.discover.outputs.batch_id }}\n",
			}},
		},
		{
			name: "integration result weakened to not cancelled",
			mutations: []workflowMutation{{
				workflow:    "chart-integration.yaml",
				old:         "    if: ${{ needs.discover-charts.result == 'success' && (needs.chart-test.result == 'success' || needs.chart-test.result == 'skipped') }}\n",
				replacement: "    if: ${{ !cancelled() }}\n",
			}},
		},
		{
			name: "chart generator source path removed",
			mutations: []workflowMutation{{
				workflow:    "orchestrator.yaml",
				old:         "      - internal/chartgen/**\n",
				replacement: "",
			}},
		},
		{
			name: "chart generator command path removed",
			mutations: []workflowMutation{{
				workflow:    "orchestrator.yaml",
				old:         "      - cmd/chart_gen.go\n",
				replacement: "",
			}},
		},
		{
			name: "chart generator workflow path removed",
			mutations: []workflowMutation{{
				workflow:    "orchestrator.yaml",
				old:         "      - .github/workflows/chart-gen.yaml\n",
				replacement: "",
			}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given a reviewed semantic bypass in an otherwise coherent graph.
			root := copyCoherentWorkflowFixture(t)
			for _, mutation := range test.mutations {
				replaceWorkflowText(t, root, mutation)
			}

			// When the coherent producer policy validates the mutated graph.
			workflows, err := loadWorkflows(root)
			require.NoError(t, err)
			violations := validateCoherentProducerChain(workflows)

			// Then the bypass fails closed.
			require.NotEmpty(t, violations)
			assert.Contains(t, violationRules(violations), RuleProducerIdentity)
		})
	}
}

func TestValidateCoherentProducerChain_rejects_contract_without_publication_id(t *testing.T) {
	// Given an otherwise coherent contract with publication identity removed.
	root := copyCoherentWorkflowFixture(t)
	replaceWorkflowText(t, root, workflowMutation{
		workflow: "chart-gen.yaml",
		old: "      publication_id:\n" +
			"        required: true\n" +
			"        type: string\n",
		replacement: "",
	})

	// When the coherent producer policy validates the producer interfaces.
	workflows, err := loadWorkflows(root)
	require.NoError(t, err)
	violations := validateCoherentProducerChain(workflows)

	// Then a distinct required publication identity is mandatory.
	require.NotEmpty(t, violations)
	assert.Contains(t, violationRules(violations), RuleProducerIdentity)
}

func TestRepositoryOrchestrator_preserves_chart_generation_push_paths(t *testing.T) {
	// Given the live orchestrator workflow.
	source := filepath.Join("..", "..", "..", ".github", "workflows", copaOrchestratorWorkflow)
	data, err := os.ReadFile(source)
	require.NoError(t, err)
	workflowPath := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(workflowPath, copaOrchestratorWorkflow), data, 0o600))
	workflows, err := loadWorkflows(workflowPath)
	require.NoError(t, err)
	orchestrator, found := findWorkflow(workflows, copaOrchestratorWorkflow)
	require.True(t, found)

	// When its push path contract is inspected.
	paths := orchestrator.Workflow.On.Push.Paths

	// Then all legacy chart generation entry points still start the exact graph.
	assert.Contains(t, paths, "internal/chartgen/**")
	assert.Contains(t, paths, "cmd/chart_gen.go")
	assert.Contains(t, paths, ".github/workflows/chart-gen.yaml")
}
