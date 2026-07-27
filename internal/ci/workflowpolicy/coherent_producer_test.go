package workflowpolicy

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateDirectory_accepts_coherent_COPA_chart_producer_graph(t *testing.T) {
	// Given the canonical Integer fixtures plus the exact COPA and chart producer chain.
	root := copyCoherentWorkflowFixture(t)

	// When workflow policy validates the complete fixture set.
	report, err := ValidateDirectory(root)

	// Then all reusable producers and the PR-safe integration gate are accepted.
	require.NoError(t, err)
	assert.Equal(t, 14, report.WorkflowCount)
}

func TestValidateDirectory_rejects_inexact_COPA_chart_producer_graph(t *testing.T) {
	tests := []struct {
		name     string
		mutation workflowMutation
	}{
		{
			name: "stale chart source",
			mutation: workflowMutation{
				workflow:    "orchestrator.yaml",
				old:         "      source_sha: ${{ needs.discover.outputs.source_sha }}\n",
				replacement: "      source_sha: ${{ github.sha }}\n",
			},
		},
		{
			name: "failed COPA producer accepted",
			mutation: workflowMutation{
				workflow:    "orchestrator.yaml",
				old:         "          needs.discover.outputs.artifact_digest != '' }}",
				replacement: "          needs.discover.outputs.artifact_digest != '' || true }}",
			},
		},
		{
			name: "chart producer omitted from integration needs",
			mutation: workflowMutation{
				workflow: "orchestrator.yaml", old: "  chart-integration:\n    needs: [discover, chart]",
				replacement: "  chart-integration:\n    needs: discover",
			},
		},
		{
			name: "artifact digest spoofed",
			mutation: workflowMutation{
				workflow:    "orchestrator.yaml",
				old:         "      artifact_digest: ${{ needs.chart.outputs.artifact_digest }}\n",
				replacement: "      artifact_digest: ${{ github.sha }}\n",
			},
		},
		{
			name: "integration source gate removed",
			mutation: workflowMutation{
				workflow:    "orchestrator.yaml",
				old:         "          needs.chart.outputs.source_sha == needs.discover.outputs.source_sha &&\n",
				replacement: "",
			},
		},
		{
			name: "cancelled chart producer accepted",
			mutation: workflowMutation{
				workflow:    "orchestrator.yaml",
				old:         "          needs.discover.outputs.artifact_digest != '' }}",
				replacement: "          needs.discover.outputs.artifact_digest != '' || !cancelled() }}",
			},
		},
		{
			name: "privileged reusable source gate removed",
			mutation: workflowMutation{
				workflow:    "chart-integration-privileged.yaml",
				old:         "    if: ${{ inputs.source_sha == github.sha }}",
				replacement: "    if: ${{ success() }}",
			},
		},
		{
			name: "scheduled latest-success correlation restored",
			mutation: workflowMutation{
				workflow: "chart-integration.yaml", old: "  pull_request:\n",
				replacement: "  pull_request:\n  workflow_run:\n    workflows: ['Chart Generation']\n    types: [completed]\n",
			},
		},
		{
			name: "producer polling restored",
			mutation: workflowMutation{
				workflow: "chart-gen.yaml", old: "    steps:\n      - uses: actions/download-artifact@",
				replacement: "    steps:\n      - run: ./verity ci workflowops wait-for-workflows orchestrator.yaml\n      - uses: actions/download-artifact@",
			},
		},
		{
			name: "additional stale artifact download",
			mutation: workflowMutation{
				workflow: "chart-gen.yaml", old: "    steps:\n      - uses: actions/download-artifact@",
				replacement: "    steps:\n      - uses: actions/download-artifact@3e5f45b2cfb9172054b4087a40e8e0b5a5461e7c\n        with:\n          name: stale-chart\n          run-id: \"12345\"\n          github-token: ${{ secrets.GITHUB_TOKEN }}\n      - uses: actions/download-artifact@",
			},
		},
		{
			name: "large manifest leaked through outputs",
			mutation: workflowMutation{
				workflow: "chart-gen.yaml", old: "      artifact_digest:\n        value: ${{ jobs.generate.outputs.artifact_digest }}\n",
				replacement: "      artifact_digest:\n        value: ${{ jobs.generate.outputs.artifact_digest }}\n      manifest:\n        value: ${{ jobs.generate.outputs.manifest }}\n",
			},
		},
		{
			name: "required producer run identity made optional",
			mutation: workflowMutation{
				workflow: "chart-gen.yaml", old: "      run_id:\n        required: true",
				replacement: "      run_id:\n        required: false",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given one exact-correlation edge is weakened.
			root := copyCoherentWorkflowFixture(t)
			replaceWorkflowText(t, root, test.mutation)

			// When the complete workflow set is validated.
			_, err := ValidateDirectory(root)

			// Then stale, failed, polling, or oversized producer contracts fail closed.
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrPolicyViolation)
			assert.ErrorContains(t, err, string(RuleProducerIdentity))
		})
	}
}

func TestValidateDirectory_keeps_chart_PR_jobs_read_only(t *testing.T) {
	// Given the coherent fixture with chart integration enabled on pull requests.
	root := copyCoherentWorkflowFixture(t)
	workflows, err := loadWorkflows(root)
	require.NoError(t, err)
	chartIntegration, found := findWorkflow(workflows, "chart-integration.yaml")
	require.True(t, found)

	// When effective workflow and job permissions are inspected.
	workflowWrites := chartIntegration.Workflow.Permissions.writeScopes()
	jobWrites := make([]permissionScope, 0)
	for _, jobName := range sortedJobNames(chartIntegration.Workflow.Jobs) {
		jobWrites = append(jobWrites, chartIntegration.Workflow.Jobs[jobName].Permissions.writeScopes()...)
	}

	// Then PR execution has no write-capable token scope.
	assert.Empty(t, workflowWrites)
	assert.Empty(t, jobWrites)
}

func TestRepositoryWorkflows_preserve_coherent_COPA_chart_producer_contract(t *testing.T) {
	// Given isolated copies of the repository workflows owned by this contract.
	root := t.TempDir()
	sourceRoot := filepath.Join("..", "..", "..", ".github", "workflows")
	for workflowName := range coherentWorkflowInputs {
		data, err := os.ReadFile(filepath.Join(sourceRoot, workflowName))
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(root, workflowName), data, 0o600))
	}
	workflows, err := loadWorkflows(root)
	require.NoError(t, err)

	// When the exact COPA and chart producer policy is evaluated.
	violations := validateCoherentProducerChain(workflows)

	// Then the live workflows match the same contract as the adversarial fixtures.
	assert.Empty(t, violations)
}

func copyCoherentWorkflowFixture(t *testing.T) string {
	t.Helper()

	root := copyWorkflowFixture(t, "valid")
	entries, err := os.ReadDir(filepath.Join("testdata", "coherent"))
	require.NoError(t, err)
	for _, entry := range entries {
		data, readErr := os.ReadFile(filepath.Join("testdata", "coherent", entry.Name()))
		require.NoError(t, readErr)
		require.NoError(t, os.WriteFile(filepath.Join(root, entry.Name()), data, 0o600))
	}
	return root
}
