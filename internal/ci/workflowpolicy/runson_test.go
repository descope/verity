package workflowpolicy

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateRunsOnRepository_acceptsProductionCanary(t *testing.T) {
	// Given the production workflow and RunsOn profile catalog.
	repositoryRoot := filepath.Join("..", "..", "..")
	workflows, err := loadWorkflows(filepath.Join(repositoryRoot, ".github", "workflows"))
	require.NoError(t, err)

	// When the RunsOn contract is evaluated.
	violations := validateRunsOnRepository(repositoryRoot, workflows)

	// Then the canary is isolated, pinned, and cache-free.
	assert.Empty(t, violations)
}

func TestValidateRunsOnRepository_rejectsActionMutation(t *testing.T) {
	// Given the production workflow with a different full-SHA RunsOn action.
	repositoryRoot := filepath.Join("..", "..", "..")
	workflows, err := loadWorkflows(filepath.Join(repositoryRoot, ".github", "workflows"))
	require.NoError(t, err)
	canary := mustWorkflow(t, workflows, runsOnSmokeWorkflowName)
	job := canary.Workflow.Jobs[runsOnCanaryJobName]
	for index := range job.Steps {
		if actionName(job.Steps[index].Uses) == runsOnActionName {
			job.Steps[index].Uses = runsOnActionName + "@0000000000000000000000000000000000000000"
		}
	}
	canary.Workflow.Jobs[runsOnCanaryJobName] = job
	workflows = replaceWorkflow(workflows, &canary)

	// When the RunsOn contract is evaluated.
	violations := validateRunsOnRepository(repositoryRoot, workflows)

	// Then only the reviewed release commit is accepted.
	require.NotEmpty(t, violations)
	assert.Contains(t, violationRules(violations), RuleRunsOnBoundary)
}

func TestValidateRunsOnRepository_rejectsSharedCacheExtra(t *testing.T) {
	// Given the production workflows with a cache-enabled canary profile.
	repositoryRoot := copyRunsOnConfig(t)
	configPath := filepath.Join(repositoryRoot, ".github", "runs-on.yml")
	data, err := os.ReadFile(configPath)
	require.NoError(t, err)
	data = append(data, []byte("    extras: [s3-cache]\n")...)
	require.NoError(t, os.WriteFile(configPath, data, 0o600))
	workflows, err := loadWorkflows(filepath.Join("..", "..", "..", ".github", "workflows"))
	require.NoError(t, err)

	// When the RunsOn contract is evaluated.
	violations := validateRunsOnRepository(repositoryRoot, workflows)

	// Then shared cache state is forbidden in the first canary.
	require.NotEmpty(t, violations)
	assert.Contains(t, violationRules(violations), RuleRunsOnBoundary)
}

func TestValidateRunsOnRepository_rejectsUndeclaredCanaryStep(t *testing.T) {
	// Given the production canary with an additional executable step.
	repositoryRoot := filepath.Join("..", "..", "..")
	workflows, err := loadWorkflows(filepath.Join(repositoryRoot, ".github", "workflows"))
	require.NoError(t, err)
	canary := mustWorkflow(t, workflows, runsOnSmokeWorkflowName)
	job := canary.Workflow.Jobs[runsOnCanaryJobName]
	job.Steps = append(job.Steps, workflowStep{Name: "Unexpected", Run: "aws s3 ls"})
	canary.Workflow.Jobs[runsOnCanaryJobName] = job
	workflows = replaceWorkflow(workflows, &canary)

	// When the RunsOn contract is evaluated.
	violations := validateRunsOnRepository(repositoryRoot, workflows)

	// Then instance-role access cannot be expanded by an undeclared step.
	require.NotEmpty(t, violations)
	assert.Contains(t, violationRules(violations), RuleRunsOnBoundary)
}

func TestValidateRunsOnRepository_rejectsOptionalIdentityInput(t *testing.T) {
	// Given the production canary with an optional AWS account input.
	repositoryRoot := filepath.Join("..", "..", "..")
	workflows, err := loadWorkflows(filepath.Join(repositoryRoot, ".github", "workflows"))
	require.NoError(t, err)
	canary := mustWorkflow(t, workflows, runsOnSmokeWorkflowName)
	input := canary.Workflow.On.DispatchInputs["expected_aws_account"]
	input.Required = false
	canary.Workflow.On.DispatchInputs["expected_aws_account"] = input
	workflows = replaceWorkflow(workflows, &canary)

	// When the RunsOn contract is evaluated.
	violations := validateRunsOnRepository(repositoryRoot, workflows)

	// Then the operator must provide an explicit account boundary.
	require.NotEmpty(t, violations)
	assert.Contains(t, violationRules(violations), RuleRunsOnBoundary)
}

func copyRunsOnConfig(t *testing.T) string {
	t.Helper()
	repositoryRoot := t.TempDir()
	configDirectory := filepath.Join(repositoryRoot, ".github")
	require.NoError(t, os.MkdirAll(configDirectory, 0o700))
	data, err := os.ReadFile(filepath.Join("..", "..", "..", ".github", "runs-on.yml"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(configDirectory, "runs-on.yml"), data, 0o600))
	return repositoryRoot
}

func mustWorkflow(t *testing.T, workflows []workflowFile, name string) workflowFile {
	t.Helper()
	workflow, exists := findWorkflow(workflows, name)
	require.True(t, exists)
	return workflow
}

func replaceWorkflow(workflows []workflowFile, replacement *workflowFile) []workflowFile {
	result := append([]workflowFile(nil), workflows...)
	for index := range result {
		if result[index].Name == replacement.Name {
			result[index] = *replacement
		}
	}
	return result
}
