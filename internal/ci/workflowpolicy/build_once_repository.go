package workflowpolicy

import (
	"fmt"
	"os"
	"path/filepath"
)

const RuleBuildOnce Rule = "build-once"

func validateBuildOnceDirectory(directory string, workflows []workflowFile) []Violation {
	if _, exists := findWorkflow(workflows, buildVerityWorkflowName); !exists {
		return nil
	}
	cleaned := filepath.Clean(directory)
	if filepath.Base(cleaned) != "workflows" || filepath.Base(filepath.Dir(cleaned)) != ".github" {
		return []Violation{buildOnceViolation(buildVerityWorkflowName, "", "build-once workflow must be validated from .github/workflows")}
	}
	root := filepath.Dir(filepath.Dir(cleaned))
	violations, err := validateBuildOnceRepository(root)
	if err != nil {
		return []Violation{buildOnceViolation(buildVerityWorkflowName, "", err.Error())}
	}
	return violations
}

func validateBuildOnceRepository(root string) ([]Violation, error) {
	workflowPath := filepath.Join(root, ".github", "workflows", "build-verity.yaml")
	workflowData, err := os.ReadFile(workflowPath)
	if err != nil {
		return nil, fmt.Errorf("read build-once workflow %q: %w", workflowPath, err)
	}
	workflowViolations, err := validateBuildOnceWorkflow(filepath.Base(workflowPath), workflowData)
	if err != nil {
		return nil, err
	}
	protectedPath := filepath.Join(root, ".github", "workflows", protectedBuildVerityWorkflowName)
	protectedData, err := os.ReadFile(protectedPath)
	if err != nil {
		return nil, fmt.Errorf("read protected build workflow %q: %w", protectedPath, err)
	}
	protectedViolations, err := validateProtectedBuildWorkflow(filepath.Base(protectedPath), protectedData)
	if err != nil {
		return nil, err
	}

	actionPath := filepath.Join(root, ".github", "actions", "setup-verity", "action.yml")
	actionData, err := os.ReadFile(actionPath)
	if err != nil {
		return nil, fmt.Errorf("read setup-verity action %q: %w", actionPath, err)
	}
	actionViolations, err := validateSetupVerityAction(filepath.Base(actionPath), actionData)
	if err != nil {
		return nil, err
	}

	violations := append([]Violation{}, workflowViolations...)
	violations = append(violations, protectedViolations...)
	return append(violations, actionViolations...), nil
}

func buildOnceViolation(name, job, detail string) Violation {
	return Violation{Rule: RuleBuildOnce, Workflow: name, Job: job, Detail: detail}
}
