package workflowpolicy

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
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

func TestValidateRunsOnRepository_rejectsVerificationContinueOnError(t *testing.T) {
	// Given the production canary with host verification allowed to fail.
	repositoryRoot := filepath.Join("..", "..", "..")
	workflows, err := loadWorkflows(filepath.Join(repositoryRoot, ".github", "workflows"))
	require.NoError(t, err)
	canary := mustWorkflow(t, workflows, runsOnSmokeWorkflowName)
	job := canary.Workflow.Jobs[runsOnCanaryJobName]
	job.Steps[4].ContinueOnError = scalarValue{set: true, value: "true"}
	canary.Workflow.Jobs[runsOnCanaryJobName] = job
	workflows = replaceWorkflow(workflows, &canary)

	// When the RunsOn contract is evaluated.
	violations := validateRunsOnRepository(repositoryRoot, workflows)

	// Then verification remains a mandatory fail-closed boundary.
	require.NotEmpty(t, violations)
	assert.Contains(t, violationRules(violations), RuleRunsOnBoundary)
}

func TestValidateRunsOnRepository_rejectsDuplicateVerificationOverride(t *testing.T) {
	// Given the production canary with a later flag weakening the disk minimum.
	repositoryRoot := filepath.Join("..", "..", "..")
	workflows, err := loadWorkflows(filepath.Join(repositoryRoot, ".github", "workflows"))
	require.NoError(t, err)
	canary := mustWorkflow(t, workflows, runsOnSmokeWorkflowName)
	job := canary.Workflow.Jobs[runsOnCanaryJobName]
	job.Steps[4].Run += " --minimum-disk-gib 1"
	canary.Workflow.Jobs[runsOnCanaryJobName] = job
	workflows = replaceWorkflow(workflows, &canary)

	// When the RunsOn contract is evaluated.
	violations := validateRunsOnRepository(repositoryRoot, workflows)

	// Then the reviewed command must match exactly.
	require.NotEmpty(t, violations)
	assert.Contains(t, violationRules(violations), RuleRunsOnBoundary)
}

func TestValidateRunsOnRepository_rejectsJobContinueOnError(t *testing.T) {
	// Given the production canary job configured to tolerate any failure.
	repositoryRoot := filepath.Join("..", "..", "..")
	workflows, err := loadWorkflows(filepath.Join(repositoryRoot, ".github", "workflows"))
	require.NoError(t, err)
	canary := mustWorkflow(t, workflows, runsOnSmokeWorkflowName)
	job := canary.Workflow.Jobs[runsOnCanaryJobName]
	job.ContinueOnError = scalarValue{set: true, value: "true"}
	canary.Workflow.Jobs[runsOnCanaryJobName] = job
	workflows = replaceWorkflow(workflows, &canary)

	// When the RunsOn contract is evaluated.
	violations := validateRunsOnRepository(repositoryRoot, workflows)

	// Then the job cannot neutralize host-verification failures.
	require.NotEmpty(t, violations)
	assert.Contains(t, violationRules(violations), RuleRunsOnBoundary)
}

func TestValidateRunsOnRepository_rejectsAmbientEnvironment(t *testing.T) {
	tests := map[string]func(*workflowFile){
		"workflow": func(canary *workflowFile) {
			canary.Workflow.Env = scalarMap{"TOKEN": testSecretExpression}
		},
		"build job": func(canary *workflowFile) {
			job := canary.Workflow.Jobs["build-verity"]
			job.Env = scalarMap{"TOKEN": testSecretExpression}
			canary.Workflow.Jobs["build-verity"] = job
		},
		"canary job": func(canary *workflowFile) {
			job := canary.Workflow.Jobs[runsOnCanaryJobName]
			job.Env = scalarMap{"TOKEN": testSecretExpression}
			canary.Workflow.Jobs[runsOnCanaryJobName] = job
		},
	}

	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			repositoryRoot := filepath.Join("..", "..", "..")
			workflows, err := loadWorkflows(filepath.Join(repositoryRoot, ".github", "workflows"))
			require.NoError(t, err)
			canary := mustWorkflow(t, workflows, runsOnSmokeWorkflowName)
			mutate(&canary)

			violations := validateRunsOnRepository(repositoryRoot, replaceWorkflow(workflows, &canary))

			require.NotEmpty(t, violations)
			assert.Contains(t, violationRules(violations), RuleRunsOnBoundary)
		})
	}
}

func TestValidateRunsOnRepository_rejectsRunsOnOutsideCanary(t *testing.T) {
	repositoryRoot := filepath.Join("..", "..", "..")
	workflows, err := loadWorkflows(filepath.Join(repositoryRoot, ".github", "workflows"))
	require.NoError(t, err)
	workflows = append(workflows, workflowFile{
		Name: "untrusted.yaml",
		Workflow: workflow{
			On: triggers{PullRequest: true},
			Jobs: map[string]workflowJob{
				"probe": {RunsOn: stringList{runsOnCanaryLabel}},
			},
		},
	})

	violations := validateRunsOnRepository(repositoryRoot, workflows)

	require.NotEmpty(t, violations)
	assert.Contains(t, violationRules(violations), RuleRunsOnBoundary)
}

func TestValidateRunsOnRepository_rejectsEnvironment(t *testing.T) {
	repositoryRoot := filepath.Join("..", "..", "..")
	workflows, err := loadWorkflows(filepath.Join(repositoryRoot, ".github", "workflows"))
	require.NoError(t, err)
	canary := mustWorkflow(t, workflows, runsOnSmokeWorkflowName)
	job := canary.Workflow.Jobs[runsOnCanaryJobName]
	job.Environment = yaml.Node{Kind: yaml.ScalarNode, Value: "production"}
	canary.Workflow.Jobs[runsOnCanaryJobName] = job

	violations := validateRunsOnRepository(repositoryRoot, replaceWorkflow(workflows, &canary))

	require.NotEmpty(t, violations)
	assert.Contains(t, violationRules(violations), RuleRunsOnBoundary)
}

func TestValidateRunsOnRepository_rejectsStrategy(t *testing.T) {
	for _, jobName := range []string{"build-verity", runsOnCanaryJobName} {
		t.Run(jobName, func(t *testing.T) {
			repositoryRoot := filepath.Join("..", "..", "..")
			workflows, err := loadWorkflows(filepath.Join(repositoryRoot, ".github", "workflows"))
			require.NoError(t, err)
			canary := mustWorkflow(t, workflows, runsOnSmokeWorkflowName)
			job := canary.Workflow.Jobs[jobName]
			job.Strategy = workflowStrategy{Present: true}
			canary.Workflow.Jobs[jobName] = job

			violations := validateRunsOnRepository(repositoryRoot, replaceWorkflow(workflows, &canary))

			require.NotEmpty(t, violations)
			assert.Contains(t, violationRules(violations), RuleRunsOnBoundary)
		})
	}
}

func TestValidateRunsOnRepository_rejectsBootstrapStepEnvironment(t *testing.T) {
	for stepIndex := range 4 {
		t.Run(string(rune('0'+stepIndex)), func(t *testing.T) {
			repositoryRoot := filepath.Join("..", "..", "..")
			workflows, err := loadWorkflows(filepath.Join(repositoryRoot, ".github", "workflows"))
			require.NoError(t, err)
			canary := mustWorkflow(t, workflows, runsOnSmokeWorkflowName)
			job := canary.Workflow.Jobs[runsOnCanaryJobName]
			job.Steps[stepIndex].Env = scalarMap{"TOKEN": testSecretExpression}
			canary.Workflow.Jobs[runsOnCanaryJobName] = job

			violations := validateRunsOnRepository(repositoryRoot, replaceWorkflow(workflows, &canary))

			require.NotEmpty(t, violations)
			assert.Contains(t, violationRules(violations), RuleRunsOnBoundary)
		})
	}
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
