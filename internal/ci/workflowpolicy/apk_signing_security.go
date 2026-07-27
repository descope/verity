package workflowpolicy

import (
	"fmt"
	"path"
	"strings"
)

//nolint:gosec // G101 false positive: this is the exact GitHub secret identifier used for policy matching, not secret material.
const apkPrivateKeySecretName = "APK_REPOSITORY_PRIVATE_KEY"

const (
	buildSiteWorkflowName = "build-site.yaml"
	buildSiteSignerJob    = "sign"
	signerPlanFile        = "signer-plan.json"
	signerResultFile      = "signer-result.json"
)

func validateAPKSigningBoundary(workflows []workflowFile) []Violation {
	var violations []Violation
	for fileIndex := range workflows {
		file := &workflows[fileIndex]
		if envContainsProductionKey(file.Workflow.Env) {
			violations = append(violations, Violation{Rule: RuleAPKSigningBoundary, Workflow: file.Name, Detail: "production signing key must not enter ambient workflow environment"})
		}
		for _, jobName := range sortedJobNames(file.Workflow.Jobs) {
			job := file.Workflow.Jobs[jobName]
			if !isAPKSignerJob(file.Name, jobName, &job) {
				continue
			}
			if envContainsProductionKey(job.Env) || envContainsProductionKey(job.Container.Env) {
				violations = append(violations, Violation{Rule: RuleAPKSigningBoundary, Workflow: file.Name, Job: jobName, Detail: "production signing key must not enter ambient job or container environment"})
			}
			if job.Container.Image != "" {
				command := append([]string{"docker", "run"}, strings.Fields(job.Container.Options)...)
				for _, volume := range job.Container.Volumes {
					command = append(command, "--volume", volume)
				}
				command = append(command, job.Container.Image)
				for _, reason := range signerContainerReasons(command) {
					violations = append(violations, Violation{Rule: RuleAPKSigningBoundary, Workflow: file.Name, Job: jobName, Detail: reason})
				}
			}
			for stepIndex := range job.Steps {
				step := &job.Steps[stepIndex]
				violations = append(violations, signerStepViolations(file.Name, jobName, step)...)
			}
		}
		violations = append(violations, signingKeyStateViolations(file)...)
	}
	return violations
}

func signingKeyStateViolations(file *workflowFile) []Violation {
	if file.Name != buildSiteWorkflowName {
		return nil
	}
	prepare, exists := file.Workflow.Jobs["prepare"]
	if !exists {
		return nil
	}
	const (
		composeCommand = "./verity ci publication compose"
		stateArgument  = "--signing-key-state ci/apk-signing-key-state.json"
	)
	composeSteps := 0
	for index := range prepare.Steps {
		step := &prepare.Steps[index]
		if !strings.Contains(step.Run, composeCommand) {
			continue
		}
		composeSteps++
		if strings.Count(step.Run, stateArgument) != 1 {
			return []Violation{{
				Rule: RuleAPKSigningBoundary, Workflow: file.Name, Job: "prepare",
				Detail: fmt.Sprintf("step %q must use the canonical APK signing key state", stepLabel(step)),
			}}
		}
	}
	if composeSteps != 2 {
		return []Violation{{
			Rule: RuleAPKSigningBoundary, Workflow: file.Name, Job: "prepare",
			Detail: fmt.Sprintf("expected two publication compose steps with canonical signing key state, found %d", composeSteps),
		}}
	}
	return nil
}

func signerStepViolations(workflowName, jobName string, step *workflowStep) []Violation {
	goOwnedBoundary := isGoOwnedSignerBoundary(workflowName, jobName, step)
	reasons := signerStepSecretViolations(step, goOwnedBoundary)
	lower := strings.ToLower(step.Run)
	if stringContainsAny(lower, "set -x", "set -o xtrace", "sh -x", "bash -x") {
		reasons = append(reasons, "xtrace is forbidden in the signing boundary")
	}
	if stringContainsAny(lower, "apk add ", "apt-get install ", "apt install ", "dnf install ", "yum install ", "pip install ", "go install ", "mise install") {
		reasons = append(reasons, "runtime installation is forbidden in the signing boundary")
	}
	for _, command := range splitShellCommands(step.Run) {
		invocation := parseShellInvocation(command)
		if invocation.executable < 0 || invocation.executable >= len(command) {
			continue
		}
		executable := path.Base(command[invocation.executable])
		if executable == "melange" || executable == "apk" {
			reasons = append(reasons, "host-resolved signing executables are forbidden")
		}
		if executable == "docker" || executable == "podman" {
			reasons = append(reasons, signerContainerReasons(command[invocation.executable:])...)
		}
	}
	violations := make([]Violation, 0, len(reasons))
	for _, reason := range reasons {
		violations = append(violations, Violation{Rule: RuleAPKSigningBoundary, Workflow: workflowName, Job: jobName, Detail: fmt.Sprintf("step %q: %s", stepLabel(step), reason)})
	}
	return violations
}

func signerStepSecretViolations(step *workflowStep, goOwnedBoundary bool) []string {
	var reasons []string
	for name, value := range step.Env {
		if strings.EqualFold(name, apkPrivateKeySecretName) || containsProductionKey(value) {
			if !goOwnedBoundary {
				reasons = append(reasons, "production signing key must not enter ambient step environment")
			}
			break
		}
	}
	if containsProductionKey(step.Run) && !goOwnedBoundary {
		reasons = append(reasons, "production signing key must not appear in command argv or Docker environment")
	}
	return reasons
}

func isGoOwnedSignerBoundary(workflowName, jobName string, step *workflowStep) bool {
	if workflowName != buildSiteWorkflowName || jobName != buildSiteSignerJob || !hasSignerSecretEnvironment(step.Env) {
		return false
	}
	return isBoundedSignerInvocation(strings.TrimSpace(step.Run))
}

func hasSignerSecretEnvironment(environment scalarMap) bool {
	if len(environment) != 1 {
		return false
	}
	value, ok := environment[apkPrivateKeySecretName]
	return ok && normalizeExpression(value) == "${{secrets."+apkPrivateKeySecretName+"}}"
}

func isBoundedSignerInvocation(statement string) bool {
	prefix := `./verity ci site-publication signer-execute ` + signerPlanFile
	if !strings.HasPrefix(statement, prefix) {
		return false
	}
	output := strings.TrimSpace(strings.TrimPrefix(statement, prefix))
	return boundedSignerMachineOutput(output)
}

func boundedSignerMachineOutput(output string) bool {
	normalized := strings.Join(strings.Fields(output), " ")
	return normalized == "--record-output "+signerResultFile+" --github-output \"$GITHUB_OUTPUT\""
}

func isAPKSignerJob(workflowName, jobName string, job *workflowJob) bool {
	if workflowName != "apk-repository.yaml" && workflowName != "build-site.yaml" {
		return false
	}
	if envContainsProductionKey(job.Env) || envContainsProductionKey(job.Container.Env) || strings.Contains(strings.ToLower(job.Container.Image), "apk-repository-signer") {
		return true
	}
	var text strings.Builder
	text.WriteString(strings.ToLower(jobName + "\n" + job.Uses))
	for stepIndex := range job.Steps {
		step := &job.Steps[stepIndex]
		if envContainsProductionKey(step.Env) {
			return true
		}
		text.WriteByte('\n')
		text.WriteString(strings.ToLower(step.Name + "\n" + step.Run + "\n" + step.Uses))
	}
	normalizedText := text.String()
	return strings.Contains(normalizedText, "sign") || strings.Contains(normalizedText, "apk-repository-signer") || strings.Contains(normalizedText, strings.ToLower(apkPrivateKeySecretName))
}

func containsProductionKey(value string) bool {
	return strings.Contains(strings.ToUpper(value), apkPrivateKeySecretName)
}

func envContainsProductionKey(environment scalarMap) bool {
	for name, value := range environment {
		if strings.EqualFold(name, apkPrivateKeySecretName) || containsProductionKey(value) {
			return true
		}
	}
	return false
}

func stringContainsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}
