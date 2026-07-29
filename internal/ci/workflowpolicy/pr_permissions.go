package workflowpolicy

import "fmt"

type prWriteGrant struct {
	workflow string
	job      string
	scope    permissionScope
}

var authorizedPRWriteGrants = map[prWriteGrant]struct{}{
	{workflow: "codeql.yaml", job: "analyze", scope: securityScope}: {},
}

func validatePRWrites(file *workflowFile) []Violation {
	if !file.Workflow.On.PullRequest && !file.Workflow.On.PullRequestTarget {
		return nil
	}
	var violations []Violation
	for _, scope := range file.Workflow.Permissions.writeScopes() {
		if !prWriteAuthorized(file.Name, "", scope) {
			violations = append(violations, prWriteViolation(file.Name, "", scope))
		}
	}
	for _, jobName := range sortedJobNames(file.Workflow.Jobs) {
		for _, scope := range file.Workflow.Jobs[jobName].Permissions.writeScopes() {
			if !prWriteAuthorized(file.Name, jobName, scope) {
				violations = append(violations, prWriteViolation(file.Name, jobName, scope))
			}
		}
	}
	return violations
}

func prWriteAuthorized(workflowName, jobName string, scope permissionScope) bool {
	_, authorized := authorizedPRWriteGrants[prWriteGrant{
		workflow: workflowName,
		job:      jobName,
		scope:    scope,
	}]
	return authorized
}

func prWriteViolation(workflowName, jobName string, scope permissionScope) Violation {
	return Violation{
		Rule: RulePRWrite, Workflow: workflowName, Job: jobName,
		Detail: fmt.Sprintf("pull-request execution is not authorized for %s: write", scope),
	}
}
