package workflowpolicy

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegerDirectWrappers_rejectCallerControlledReusableMode(t *testing.T) {
	tests := []string{"integer-orchestrator.yaml", "integer-build-image.yaml"}
	for _, name := range tests {
		t.Run(name, func(t *testing.T) {
			// Given: a public Integer entry point parsed from the repository.
			workflow := readRepositoryIntegerWorkflow(t, name)

			// When: its trigger surface and builder routing are inspected.
			_, exposesModeSelector := workflow.On.WorkflowInputs["reusable_call"]
			builder := workflow.Jobs["build-verity"]

			// Then: an actual workflow_call cannot pass reusable_call=false to
			// select the direct self-building path.
			assert.False(t, workflow.On.WorkflowCall, "public direct wrapper must not accept workflow_call")
			assert.False(t, exposesModeSelector, "reusable_call=false exposes the self-building path")
			assert.NotContains(t, strings.ReplaceAll(builder.If, " ", ""), "inputs.reusable_call", "caller input controls build-verity")
		})
	}
}

func TestIntegerStrictReusableWorkflows_rejectEveryIncompleteCoordinateState(t *testing.T) {
	strictWorkflows := []string{
		"integer-orchestrator-reusable.yaml",
		"integer-build-shard.yaml",
		"integer-build-image-reusable.yaml",
	}
	for _, name := range strictWorkflows {
		t.Run(name, func(t *testing.T) {
			// Given: a parsed strict reusable implementation and its real guard.
			workflow := readRepositoryIntegerWorkflow(t, name)
			validator, exists := workflow.Jobs["validate-coordinates"]
			require.True(t, exists)
			guard := integerCoordinateGuardStep(t, validator.Steps)

			// When/Then: every one of the 16 coordinate states is evaluated.
			for mask := range 1 << len(integerCoordinateInputs) {
				values := make(map[string]string, len(integerCoordinateInputs))
				for index, coordinate := range integerCoordinateInputs {
					if mask&(1<<index) != 0 {
						values[coordinate] = "set"
					}
				}
				got := evaluateIntegerCoordinateGuard(t, guard.If, values)
				assert.Equal(t, mask != 15, got, "mask=%04b", mask)
			}
			assert.Equal(t, "exit 1", strings.TrimSpace(guard.Run))
			assert.True(t, workflow.On.WorkflowCall)
			assert.False(t, workflow.On.WorkflowDispatch)
			assert.False(t, workflow.On.Push.Present)
			assert.False(t, workflow.On.Schedule)
			_, exposesModeSelector := workflow.On.WorkflowInputs["reusable_call"]
			assert.False(t, exposesModeSelector)
			for _, coordinate := range integerCoordinateInputs {
				input, present := workflow.On.WorkflowInputs[coordinate]
				require.True(t, present, coordinate)
				assert.True(t, input.Required, coordinate)
				assert.Equal(t, "string", input.Type, coordinate)
			}
		})
	}
}

func TestIntegerParsedTopology_buildsBeforeFanout_andStrictWorkflowsOnlyConsume(t *testing.T) {
	wrappers := []struct {
		name           string
		implementation string
		job            string
	}{
		{name: "integer-orchestrator.yaml", implementation: integerOrchestratorReference, job: "orchestrate"},
		{name: "integer-build-image.yaml", implementation: integerImageWorkflowReference, job: "build"},
	}
	for _, wrapper := range wrappers {
		workflow := readRepositoryIntegerWorkflow(t, wrapper.name)
		builderCount := 0
		for _, job := range workflow.Jobs {
			if job.Uses == integerBuildVerityReference {
				builderCount++
			}
		}
		implementation := workflow.Jobs[wrapper.job]
		assert.Equal(t, 1, builderCount, wrapper.name)
		assert.Equal(t, wrapper.implementation, implementation.Uses)
		assert.Equal(t, []string{"build-verity"}, []string(implementation.Needs))
	}

	matrixJobs := []struct {
		workflow string
		job      string
		needs    string
	}{
		{workflow: "integer-orchestrator-reusable.yaml", job: "build-shards", needs: "plan"},
		{workflow: "integer-build-shard.yaml", job: "build", needs: "validate-coordinates"},
		{workflow: "integer-build-image-reusable.yaml", job: "melange-build", needs: "melange-prep"},
	}
	for _, matrix := range matrixJobs {
		workflow := readRepositoryIntegerWorkflow(t, matrix.workflow)
		job := workflow.Jobs[matrix.job]
		assert.NotZero(t, job.Strategy.Matrix.Kind, fmt.Sprintf("%s:%s", matrix.workflow, matrix.job))
		assert.Contains(t, []string(job.Needs), matrix.needs)
	}

	for _, name := range []string{"integer-orchestrator-reusable.yaml", "integer-build-shard.yaml", "integer-build-image-reusable.yaml"} {
		workflow := readRepositoryIntegerWorkflow(t, name)
		for jobName, job := range workflow.Jobs {
			assert.NotEqual(t, integerBuildVerityReference, job.Uses, name+":"+jobName)
		}
	}
}

func TestIntegerStrictReusableWorkflows_setupVerityBeforeEveryInvocation(t *testing.T) {
	for _, name := range []string{"integer-orchestrator-reusable.yaml", "integer-build-shard.yaml", "integer-build-image-reusable.yaml"} {
		workflow := readRepositoryIntegerWorkflow(t, name)
		for jobName, job := range workflow.Jobs {
			setupIndex := -1
			for index, step := range job.Steps {
				if step.Uses == "./.github/actions/setup-verity" {
					setupIndex = index
				}
				if strings.Contains(step.Run, "./verity") {
					assert.GreaterOrEqual(t, setupIndex, 0, name+":"+jobName+":"+step.Name)
					assert.Less(t, setupIndex, index, name+":"+jobName+":"+step.Name)
				}
			}
		}
	}
}

func TestValidateIntegerStructuralTopology_rejectsPermissionMarkerSubstringSpoof(t *testing.T) {
	// Given: the exact public wrapper is redirected to a nonexistent path that
	// still contains the broad integer-build- permission marker substring.
	workflows := repositoryIntegerWorkflowFiles(t)
	index := slices.IndexFunc(workflows, func(file workflowFile) bool {
		return file.Name == "integer-orchestrator.yaml"
	})
	require.NotEqual(t, -1, index)
	wrapper := workflows[index].Workflow.Jobs["orchestrate"]
	wrapper.Uses = "./.github/workflows/integer-build-decoy.yaml"
	workflows[index].Workflow.Jobs["orchestrate"] = wrapper

	// When: exact Integer topology policy is evaluated.
	violations := validateIntegerStructuralTopology(workflows)

	// Then: substring permission justification cannot spoof the trusted path.
	require.NotEmpty(t, violations)
	assert.Contains(t, violations[0].Detail, "strict implementation")
}

func integerCoordinateGuardStep(t *testing.T, steps []workflowStep) workflowStep {
	t.Helper()
	for stepIndex := range steps {
		step := &steps[stepIndex]
		if step.Name == "Reject incomplete Verity coordinates" {
			return *step
		}
	}
	require.FailNow(t, "coordinate guard step is missing")
	return workflowStep{}
}

func evaluateIntegerCoordinateGuard(t *testing.T, expression string, values map[string]string) bool {
	t.Helper()
	clauses := strings.Split(strings.Join(strings.Fields(expression), ""), "||")
	result := false
	for _, clause := range clauses {
		parts := strings.Split(clause, "==''")
		require.Len(t, parts, 2, clause)
		name := strings.TrimPrefix(parts[0], "inputs.")
		require.Contains(t, integerCoordinateInputs, name)
		result = result || values[name] == ""
	}
	return result
}
