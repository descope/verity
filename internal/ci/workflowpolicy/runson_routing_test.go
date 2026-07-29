package workflowpolicy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateRunsOnRepository_rejectsProductionRouteMutation(t *testing.T) {
	repositoryRoot := filepath.Join("..", "..", "..")
	workflows, err := loadWorkflows(filepath.Join(repositoryRoot, ".github", "workflows"))
	require.NoError(t, err)
	workflow := mustWorkflow(t, workflows, "ci.yaml")
	job := workflow.Workflow.Jobs["test"]
	job.RunsOn = stringList{"runs-on=${{ github.run_id }}/runner=integer-x64"}
	workflow.Workflow.Jobs["test"] = job

	violations := validateRunsOnRepository(repositoryRoot, replaceWorkflow(workflows, &workflow))

	require.NotEmpty(t, violations)
	assert.Contains(t, violationRules(violations), RuleRunsOnBoundary)
}

func TestValidateRunsOnRepository_rejectsProductionRouteFallback(t *testing.T) {
	repositoryRoot := filepath.Join("..", "..", "..")
	workflows, err := loadWorkflows(filepath.Join(repositoryRoot, ".github", "workflows"))
	require.NoError(t, err)
	workflow := mustWorkflow(t, workflows, "ci.yaml")
	job := workflow.Workflow.Jobs["test"]
	job.RunsOn = stringList{"ubuntu-latest"}
	workflow.Workflow.Jobs["test"] = job

	violations := validateRunsOnRepository(repositoryRoot, replaceWorkflow(workflows, &workflow))

	require.NotEmpty(t, violations)
	assert.Contains(t, violationRules(violations), RuleRunsOnBoundary)
}

func TestValidateRunsOnRepository_rejectsProductionRouteWithoutTrustGuard(t *testing.T) {
	repositoryRoot := filepath.Join("..", "..", "..")
	workflows, err := loadWorkflows(filepath.Join(repositoryRoot, ".github", "workflows"))
	require.NoError(t, err)
	workflow := mustWorkflow(t, workflows, "ci.yaml")
	job := workflow.Workflow.Jobs["test"]
	job.If = "needs.changes.outputs.tests == 'true'"
	workflow.Workflow.Jobs["test"] = job

	violations := validateRunsOnRepository(repositoryRoot, replaceWorkflow(workflows, &workflow))

	require.NotEmpty(t, violations)
	assert.Contains(t, violationRules(violations), RuleRunsOnBoundary)
}

func TestValidateRunsOnRepository_rejectsProductionRouteWithoutAction(t *testing.T) {
	repositoryRoot := filepath.Join("..", "..", "..")
	workflows, err := loadWorkflows(filepath.Join(repositoryRoot, ".github", "workflows"))
	require.NoError(t, err)
	workflow := mustWorkflow(t, workflows, "ci.yaml")
	job := workflow.Workflow.Jobs["test"]
	steps := make([]workflowStep, 0, len(job.Steps)-1)
	for index := range job.Steps {
		if actionName(job.Steps[index].Uses) != runsOnActionName {
			steps = append(steps, job.Steps[index])
		}
	}
	job.Steps = steps
	workflow.Workflow.Jobs["test"] = job

	violations := validateRunsOnRepository(repositoryRoot, replaceWorkflow(workflows, &workflow))

	require.NotEmpty(t, violations)
	assert.Contains(t, violationRules(violations), RuleRunsOnBoundary)
}

func TestValidateRunsOnRepository_rejectsProfileCapacityMutation(t *testing.T) {
	repositoryRoot := copyRunsOnConfig(t)
	configPath := filepath.Join(repositoryRoot, ".github", "runs-on.yml")
	data, err := os.ReadFile(configPath)
	require.NoError(t, err)
	mutated := strings.Replace(string(data), "  ci-large-x64:\n    family: [c8i, m8i]\n    cpu: 16", "  ci-large-x64:\n    family: [c8i, m8i]\n    cpu: 8", 1)
	require.NotEqual(t, string(data), mutated)
	require.NoError(t, os.WriteFile(configPath, []byte(mutated), 0o600))
	workflows, err := loadWorkflows(filepath.Join("..", "..", "..", ".github", "workflows"))
	require.NoError(t, err)

	violations := validateRunsOnRepository(repositoryRoot, workflows)

	require.NotEmpty(t, violations)
	assert.Contains(t, violationRules(violations), RuleRunsOnBoundary)
}

func TestRunsOnCatalog_containsPRIntegerArchitectureProfiles(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", ".github", "runs-on.yml"))
	require.NoError(t, err)
	catalog, err := decodeRunsOnCatalog(data)
	require.NoError(t, err)

	for _, architecture := range []string{"amd64", "arm64"} {
		_, exists := catalog.Runners["integer-"+architecture]
		assert.True(t, exists, architecture)
	}
}

func TestBuildVerityWorkflows_routeTrustedCompilationToRunsOn(t *testing.T) {
	repositoryRoot := filepath.Join("..", "..", "..")
	workflows, err := loadWorkflows(filepath.Join(repositoryRoot, ".github", "workflows"))
	require.NoError(t, err)

	buildVerity := mustWorkflow(t, workflows, "build-verity.yaml")
	require.Equal(t, stringList{runsOnCILargeX64Route}, buildVerity.Workflow.Jobs["build"].RunsOn)

	protectedBuild := mustWorkflow(t, workflows, "build-verity-protected.yaml")
	require.Equal(t,
		stringList{runsOnCILargeX64Route},
		protectedBuild.Workflow.Jobs["build"].RunsOn,
	)
}

func TestRunsOnRoutes_doNotDependOnDefaultBranchProfiles(t *testing.T) {
	repositoryRoot := filepath.Join("..", "..", "..")
	workflows, err := loadWorkflows(filepath.Join(repositoryRoot, ".github", "workflows"))
	require.NoError(t, err)

	for _, workflow := range workflows {
		for jobName, job := range workflow.Workflow.Jobs {
			for _, label := range job.RunsOn {
				assert.NotContains(t, label, "/runner=", workflow.Name+":"+jobName)
			}
		}
	}
}

func TestRepositoryWorkflows_doNotUseGitHubHostedRunners(t *testing.T) {
	repositoryRoot := filepath.Join("..", "..", "..")
	workflows, err := loadWorkflows(filepath.Join(repositoryRoot, ".github", "workflows"))
	require.NoError(t, err)

	for _, workflow := range workflows {
		for jobName, job := range workflow.Workflow.Jobs {
			for _, label := range job.RunsOn {
				assert.False(t, isGitHubHostedRunner(label), workflow.Name+":"+jobName)
			}
		}
	}
}
