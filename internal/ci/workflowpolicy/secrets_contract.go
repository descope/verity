package workflowpolicy

import (
	"path/filepath"
	"strings"
)

func validateReusableJobSecrets(workflows []workflowFile) []Violation {
	var violations []Violation
	for fileIndex := range workflows {
		file := &workflows[fileIndex]
		for _, jobName := range sortedJobNames(file.Workflow.Jobs) {
			job := file.Workflow.Jobs[jobName]
			if !job.Secrets.set || job.Secrets.inherit || len(job.Secrets.values) == 0 {
				continue
			}
			workflowName, local := localReusableWorkflowName(job.Uses)
			if !local {
				continue
			}
			called, exists := findWorkflow(workflows, workflowName)
			if !exists || !called.Workflow.On.WorkflowCall {
				violations = append(violations, undeclaredSecretViolation(file.Name, jobName, "called workflow contract is missing"))
				continue
			}
			for name := range job.Secrets.values {
				if _, declared := called.Workflow.On.WorkflowSecrets[name]; !declared {
					violations = append(violations, undeclaredSecretViolation(file.Name, jobName, "secret "+name+" is not declared by "+workflowName))
				}
			}
		}
	}
	return violations
}

func localReusableWorkflowName(reference string) (string, bool) {
	const prefix = "./.github/workflows/"
	if !strings.HasPrefix(reference, prefix) {
		return "", false
	}
	name := strings.TrimPrefix(reference, prefix)
	if name == "" || filepath.Base(name) != name {
		return "", false
	}
	extension := filepath.Ext(name)
	return name, extension == ".yaml" || extension == ".yml"
}

func undeclaredSecretViolation(workflowName, jobName, detail string) Violation {
	return Violation{
		Rule: RuleLeastPrivilege, Workflow: workflowName, Job: jobName,
		Detail: detail,
	}
}
