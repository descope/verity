package workflowpolicy

import (
	"fmt"
	"strings"
)

const (
	approvedAPKArtifactName       = "apk-repository-${{ inputs.batch_id }}-${{ inputs.shard }}"
	integerOrchestratorReference  = "./.github/workflows/integer-orchestrator-reusable.yaml"
	integerShardWorkflowReference = "./.github/workflows/integer-build-shard.yaml"
	integerImageWorkflowReference = "./.github/workflows/integer-build-image-reusable.yaml"
)

func validateIntegerContract(workflows []workflowFile) []Violation {
	violations := make([]Violation, 0, 8)
	violations = append(violations, validateIntegerTriggers(workflows)...)
	violations = append(violations, validateIntegerExactWorkflows(workflows)...)
	violations = append(violations, validateCrossRunDownloads(workflows)...)
	return violations
}

func validateIntegerTriggers(workflows []workflowFile) []Violation {
	file, ok := findWorkflow(workflows, "integer-orchestrator.yaml")
	if !ok {
		return []Violation{{
			Rule: RuleIntegerTriggers, Workflow: "integer-orchestrator.yaml",
			Detail: "required workflow is missing",
		}}
	}
	if !file.Workflow.On.Push.Present {
		return []Violation{{
			Rule: RuleIntegerTriggers, Workflow: file.Name,
			Detail: "Integer orchestrator must have a production push trigger",
		}}
	}

	paths := make(map[string]struct{}, len(file.Workflow.On.Push.Paths))
	for _, path := range file.Workflow.On.Push.Paths {
		paths[strings.TrimSpace(path)] = struct{}{}
	}
	var violations []Violation
	if _, broadImages := paths["images/**"]; !broadImages {
		_, topLevelImages := paths["images/*.yaml"]
		_, nestedImages := paths["images/**/*.yaml"]
		if !topLevelImages || !nestedImages {
			violations = append(violations, Violation{
				Rule: RuleIntegerTriggers, Workflow: file.Name,
				Detail: "push paths do not cover image definitions",
			})
		}
	}
	requirements := []struct {
		name         string
		alternatives []string
	}{
		{name: "Integer config", alternatives: []string{"integer.yaml"}},
		{name: "bespoke recipes", alternatives: []string{"packages/bespoke/**", "packages/**"}},
		{name: "shared package pipelines", alternatives: []string{"packages/pipelines/**", "packages/**"}},
		{name: "package overrides", alternatives: []string{"packages/overrides/**", "packages/**"}},
		{name: "upstream package lock", alternatives: []string{"packages/upstream.lock.json", "packages/**"}},
		{name: "root Go code", alternatives: []string{"*.go"}},
		{name: "CI planning code", alternatives: []string{"internal/ci/**", "internal/**", "**/*.go"}},
		{name: "Integer code", alternatives: []string{"internal/integer/**", "internal/**", "**/*.go"}},
		{name: "CLI code", alternatives: []string{"cmd/*.go", "cmd/**", "**/*.go"}},
		{name: "Go module", alternatives: []string{"go.mod"}},
		{name: "Go dependency lock", alternatives: []string{"go.sum"}},
		{name: "tool versions", alternatives: []string{"mise.toml"}},
		{name: "tool lock", alternatives: []string{"mise.lock"}},
		{name: "orchestrator workflow", alternatives: []string{".github/workflows/integer-orchestrator.yaml", ".github/workflows/**"}},
		{name: "orchestrator implementation", alternatives: []string{".github/workflows/integer-orchestrator-reusable.yaml", ".github/workflows/**"}},
		{name: "shard workflow", alternatives: []string{".github/workflows/integer-build-shard.yaml", ".github/workflows/**"}},
		{name: "image workflow", alternatives: []string{".github/workflows/integer-build-image.yaml", ".github/workflows/**"}},
		{name: "image implementation", alternatives: []string{".github/workflows/integer-build-image-reusable.yaml", ".github/workflows/**"}},
	}
	for _, requirement := range requirements {
		if !containsAny(paths, requirement.alternatives) {
			violations = append(violations, Violation{
				Rule: RuleIntegerTriggers, Workflow: file.Name,
				Detail: "push paths do not cover " + requirement.name,
			})
		}
	}
	return violations
}

func containsAny(values map[string]struct{}, candidates []string) bool {
	for _, candidate := range candidates {
		if _, ok := values[candidate]; ok {
			return true
		}
	}
	return false
}

func validateWorkflowCallIdentityInputs(file *workflowFile, names []string) []Violation {
	var violations []Violation
	for _, name := range names {
		input, present := file.Workflow.On.WorkflowInputs[name]
		if !file.Workflow.On.WorkflowCall || !present || !input.Required || input.Type != workflowInputStringType {
			violations = append(violations, Violation{
				Rule: RuleProducerIdentity, Workflow: file.Name,
				Detail: fmt.Sprintf("workflow_call input %q must be a required string", name),
			})
		}
	}
	return violations
}

func normalizeExpression(value string) string {
	return strings.ReplaceAll(strings.TrimSpace(value), " ", "")
}
