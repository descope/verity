package workflowpolicy

import "strings"

func jobUsesSecretContextOnPullRequest(workflowEnvironment scalarMap, job *workflowJob) bool {
	if scalarMapUsesPrivilegedSecret(workflowEnvironment) || scalarMapUsesPrivilegedSecret(job.Env) ||
		scalarMapUsesPrivilegedSecret(job.With) || scalarMapUsesPrivilegedSecret(job.Outputs) ||
		scalarMapUsesPrivilegedSecret(job.Container.Env) || job.Secrets.set {
		return true
	}
	for _, service := range job.Services {
		if scalarMapUsesPrivilegedSecret(service.Env) {
			return true
		}
	}
	for stepIndex := range job.Steps {
		step := &job.Steps[stepIndex]
		if containsSecretExpression(step.If) {
			return true
		}
		if gateExcludesPullRequest(step.If) {
			continue
		}
		if scalarMapUsesPrivilegedSecret(step.Env) || scalarMapUsesPrivilegedSecret(step.With) || containsPrivilegedSecretExpression(step.Run) {
			return true
		}
	}
	return false
}

func jobUsesSecretContext(workflowEnvironment scalarMap, job *workflowJob) bool {
	if scalarMapUsesSecret(workflowEnvironment) || scalarMapUsesSecret(job.Env) ||
		scalarMapUsesSecret(job.With) || scalarMapUsesSecret(job.Outputs) ||
		scalarMapUsesSecret(job.Container.Env) || job.Secrets.set {
		return true
	}
	for _, service := range job.Services {
		if scalarMapUsesSecret(service.Env) {
			return true
		}
	}
	for stepIndex := range job.Steps {
		step := &job.Steps[stepIndex]
		if containsSecretExpression(step.If) || scalarMapUsesSecret(step.Env) ||
			scalarMapUsesSecret(step.With) || containsSecretExpression(step.Run) {
			return true
		}
	}
	return false
}

func scalarMapUsesPrivilegedSecret(values scalarMap) bool {
	for _, value := range values {
		if containsPrivilegedSecretExpression(value) {
			return true
		}
	}
	return false
}

func containsPrivilegedSecretExpression(value string) bool {
	normalized := strings.ToLower(normalizeExpression(value))
	return normalized != "${{secrets.github_token}}" && containsSecretExpression(value)
}

func scalarMapUsesSecret(values scalarMap) bool {
	for _, value := range values {
		if containsSecretExpression(value) {
			return true
		}
	}
	return false
}

func containsSecretExpression(value string) bool {
	normalized := strings.ToLower(normalizeExpression(value))
	return strings.Contains(normalized, "${{secrets.") || strings.Contains(normalized, "${{secrets[")
}
