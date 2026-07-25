package workflowpolicy

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRepositoryChartIntegration_uses_typed_result_aggregation_without_inline_shell_policy(t *testing.T) {
	// Given the five live Todo 8 workflows.
	root := copyLiveCoherentWorkflows(t)
	workflows, err := loadWorkflows(root)
	require.NoError(t, err)

	// When both actual integration output jobs and their shell ownership are validated.
	violations := validateCoherentProducerChain(workflows)
	for _, workflowName := range []string{chartIntegrationWorkflow, privilegedChartWorkflow} {
		chart, found := findWorkflow(workflows, workflowName)
		require.True(t, found)
		resultName, resultJob, found := integrationOutputJob(&chart)
		require.True(t, found)
		aggregateFound := false
		for _, step := range resultJob.Steps {
			if step.ID == "aggregate" && strings.Contains(step.Run, "./verity ci workflowops aggregate-chart-results") {
				aggregateFound = true
			}
			assert.Empty(t, workflowLogicViolation(step.Run, step.Shell), "%s:%s", workflowName, resultName)
		}
		assert.True(t, aggregateFound, "%s:%s", workflowName, resultName)
	}

	// Then both result jobs are bound to the typed Go aggregator and contain no shell policy.
	assert.Empty(t, violations)
}

func TestValidateCoherentProducerChain_rejects_inline_chart_result_policy(t *testing.T) {
	for _, workflowName := range []string{chartIntegrationWorkflow, privilegedChartWorkflow} {
		t.Run(workflowName, func(t *testing.T) {
			// Given either live result command replaced by inline shell success policy.
			root := copyLiveCoherentWorkflows(t)
			replaceWorkflowText(t, root, workflowMutation{
				workflow:    workflowName,
				old:         "./verity ci workflowops aggregate-chart-results",
				replacement: "bash -c 'if [[ \"$TEST_RESULT\" != \"success\" ]]; then exit 1; fi'",
			})
			workflows, err := loadWorkflows(root)
			require.NoError(t, err)

			// When coherent identity and Go-owned logic policies evaluate the actual job.
			violations := append(validateCoherentProducerChain(workflows), validateGoOwnedLogic(workflows)...)

			// Then the inline result policy fails closed.
			require.NotEmpty(t, violations)
			assert.Contains(t, violationRules(violations), RuleGoOwnedLogic)
		})
	}
}

func TestValidateCoherentProducerChain_rejects_inexact_chart_result_declarations(t *testing.T) {
	tests := []struct {
		name        string
		old         string
		replacement string
	}{
		{
			name:        "missing chart result",
			old:         "          --result chart-test=${{ needs.chart-test.result }}\n",
			replacement: "",
		},
		{
			name:        "duplicate chart result",
			old:         "          --result chart-test=${{ needs.chart-test.result }}\n",
			replacement: "          --result chart-test=${{ needs.chart-test.result }}\n          --result chart-test=skipped\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given the live command with an incomplete or duplicate declared result set.
			root := copyLiveCoherentWorkflows(t)
			replaceWorkflowText(t, root, workflowMutation{
				workflow: "chart-integration.yaml", old: test.old, replacement: test.replacement,
			})
			workflows, err := loadWorkflows(root)
			require.NoError(t, err)

			// When the actual chart result command contract is validated.
			violations := validateCoherentProducerChain(workflows)

			// Then inexact result declarations fail closed.
			require.NotEmpty(t, violations)
			assert.Contains(t, violationRules(violations), RuleProducerIdentity)
		})
	}
}
