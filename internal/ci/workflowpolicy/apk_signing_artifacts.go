package workflowpolicy

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

var immutableDigestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

var setupVerityIdentityInputs = []string{"artifact-name", "artifact-digest", "source-sha", "build-key"}

type signerActivation struct {
	file    *workflowFile
	jobName string
	job     *workflowJob
}

func validateMelangeArtifactPaths(workflows []workflowFile) []Violation {
	var violations []Violation
	for fileIndex := range workflows {
		file := &workflows[fileIndex]
		for _, jobName := range sortedJobNames(file.Workflow.Jobs) {
			job := file.Workflow.Jobs[jobName]
			for stepIndex := range job.Steps {
				step := &job.Steps[stepIndex]
				if actionName(step.Uses) != "actions/upload-artifact" {
					continue
				}
				for artifactPath := range strings.FieldsSeq(step.With["path"]) {
					if privateKeyCapableArtifactPath(artifactPath) {
						violations = append(violations, Violation{Rule: RulePrivateKeyArtifact, Workflow: file.Name, Job: jobName, Detail: fmt.Sprintf("artifact path %q can contain ephemeral private keys", artifactPath)})
					}
				}
			}
		}
	}
	return violations
}

func validateSignerProvenance(workflows []workflowFile) []Violation {
	var violations []Violation
	for fileIndex := range workflows {
		file := &workflows[fileIndex]
		for _, jobName := range sortedJobNames(file.Workflow.Jobs) {
			job := file.Workflow.Jobs[jobName]
			activation := signerActivation{file: file, jobName: jobName, job: &job}
			signingJob := isSigningJob(file.Name, jobName, &job)
			for stepIndex := range job.Steps {
				step := &job.Steps[stepIndex]
				if step.Uses != "./.github/actions/setup-verity" {
					continue
				}
				digest := strings.TrimSpace(step.With["artifact-digest"])
				if !completeSetupVerityIdentity(step) || !activation.acceptsAttestationMode(stepIndex) || signingJob && !immutableSignerDigest(digest) {
					violations = append(violations, Violation{Rule: RuleSignerProvenance, Workflow: file.Name, Job: jobName, Detail: "setup-verity requires exact artifact identity; protected, write-capable, secret-bearing, and signing jobs require attestation"})
				}
			}
		}
	}
	return violations
}

func jobRequiresSignerProvenance(file *workflowFile, jobName string, job *workflowJob) bool {
	activation := signerActivation{file: file, jobName: jobName, job: job}
	return !jobRunsOnlyOnPullRequest(file.Workflow.On, job.If) || !activation.safeOnPullRequest()
}

func pullRequestOnly(events triggers) bool {
	return events.PullRequest && !events.PullRequestTarget && !events.Push.Present &&
		!events.Schedule && !events.WorkflowCall && !events.WorkflowDispatch &&
		!events.WorkflowRun && !events.OtherEvent
}

func explicitlyReadOnly(workflowPermissions, jobPermissions permissions) bool {
	effective := workflowPermissions
	if jobPermissions.declared {
		effective = jobPermissions
	}
	return effective.declared && len(effective.writeScopes()) == 0
}

func completeSetupVerityIdentity(step *workflowStep) bool {
	for _, input := range setupVerityIdentityInputs {
		if strings.TrimSpace(step.With[input]) == "" {
			return false
		}
	}
	return true
}

func (activation signerActivation) acceptsAttestationMode(stepIndex int) bool {
	value := strings.TrimSpace(activation.job.Steps[stepIndex].With["verify-attestation"])
	if strings.EqualFold(value, "true") {
		return true
	}
	if activation.exactAttestationProducer(stepIndex) {
		return value == "" || strings.EqualFold(value, "false")
	}
	if activation.exactReadOnlySameRunActivation(stepIndex) {
		return value == "" || strings.EqualFold(value, "false")
	}
	if gatesEquivalent(value, "github.event_name != 'pull_request'") {
		return !jobMayRunOnPullRequest(activation.file.Workflow.On, activation.job.If) || activation.safeOnPullRequest()
	}
	return !jobRequiresSignerProvenance(activation.file, activation.jobName, activation.job) &&
		(value == "" || strings.EqualFold(value, "false"))
}

func (activation signerActivation) safeOnPullRequest() bool {
	return explicitlyReadOnly(activation.file.Workflow.Permissions, activation.job.Permissions) &&
		!jobUsesSecretContextOnPullRequest(activation.file.Workflow.Env, activation.job) &&
		!jobSignsOnPullRequest(activation.file.Name, activation.jobName, activation.job)
}

func (activation signerActivation) exactAttestationProducer(stepIndex int) bool {
	if activation.file.Name != buildVerityWorkflowName || activation.jobName != "attest" ||
		!gatesEquivalent(activation.job.If, protectedAttestationIf) || stepIndex+1 >= len(activation.job.Steps) {
		return false
	}
	next := &activation.job.Steps[stepIndex+1]
	return actionName(next.Uses) == "actions/attest-build-provenance" && strings.TrimSpace(next.With["subject-path"]) == "verity"
}

func jobRunsOnlyOnPullRequest(events triggers, jobIf string) bool {
	return pullRequestOnly(events) || gateRequiresPullRequest(jobIf)
}

func jobMayRunOnPullRequest(events triggers, jobIf string) bool {
	return (events.PullRequest || events.WorkflowCall) && !gateExcludesPullRequest(jobIf)
}

func gateRequiresPullRequest(value string) bool {
	expression, err := parseGateExpression(value)
	return err == nil && expressionRequiresPullRequest(expression)
}

func gateExcludesPullRequest(value string) bool {
	expression, err := parseGateExpression(value)
	return err == nil && expressionExcludesPullRequest(expression)
}

func expressionRequiresPullRequest(expression gateExpression) bool {
	switch expression.kind {
	case gateExpressionEqual:
		return pullRequestEventComparison(expression)
	case gateExpressionNot:
		return len(expression.children) == 1 && expression.children[0].kind == gateExpressionNotEqual && pullRequestEventComparison(expression.children[0])
	case gateExpressionAnd:
		return slices.ContainsFunc(expression.children, expressionRequiresPullRequest)
	case gateExpressionOr:
		return len(expression.children) > 0 && allGateChildren(expression.children, expressionRequiresPullRequest)
	default:
		return false
	}
}

func expressionExcludesPullRequest(expression gateExpression) bool {
	switch expression.kind {
	case gateExpressionNotEqual:
		return pullRequestEventComparison(expression)
	case gateExpressionNot:
		return len(expression.children) == 1 && expression.children[0].kind == gateExpressionEqual && pullRequestEventComparison(expression.children[0])
	case gateExpressionAnd:
		return slices.ContainsFunc(expression.children, expressionExcludesPullRequest)
	case gateExpressionOr:
		return len(expression.children) > 0 && allGateChildren(expression.children, expressionExcludesPullRequest)
	default:
		return false
	}
}

func pullRequestEventComparison(expression gateExpression) bool {
	if len(expression.children) != 2 {
		return false
	}
	left := expression.children[0]
	right := expression.children[1]
	return left.kind == gateExpressionAtom && right.kind == gateExpressionAtom &&
		((left.value == "github.event_name" && right.value == "string:pull_request") ||
			(left.value == "string:pull_request" && right.value == "github.event_name"))
}

func allGateChildren(children []gateExpression, predicate func(gateExpression) bool) bool {
	for _, child := range children {
		if !predicate(child) {
			return false
		}
	}
	return true
}

func isSigningJob(workflowName, jobName string, job *workflowJob) bool {
	if isAPKSignerJob(workflowName, jobName, job) {
		return true
	}
	for stepIndex := range job.Steps {
		lower := strings.ToLower(job.Steps[stepIndex].Run)
		if stringContainsAny(lower, "cosign sign", " signer-execute ", "apk-repository sign", "melange sign") {
			return true
		}
	}
	return false
}

func jobSignsOnPullRequest(workflowName, jobName string, job *workflowJob) bool {
	if isAPKSignerJob(workflowName, jobName, job) {
		return true
	}
	for stepIndex := range job.Steps {
		step := &job.Steps[stepIndex]
		if gateExcludesPullRequest(step.If) {
			continue
		}
		lower := strings.ToLower(step.Run)
		if stringContainsAny(lower, "cosign sign", " signer-execute ", "apk-repository sign", "melange sign") {
			return true
		}
	}
	return false
}

func privateKeyCapableArtifactPath(value string) bool {
	cleaned := strings.ToLower(strings.Trim(strings.TrimSpace(value), "'\""))
	if cleaned == "melange-work" || cleaned == "melange-work/" || strings.Contains(cleaned, "melange-work/**") {
		return true
	}
	return stringContainsAny(cleaned, "melange.rsa", "private", "*.key", "*.pem", "*.p12", "*.pfx") || strings.HasSuffix(cleaned, ".rsa") || strings.HasSuffix(cleaned, ".key")
}

func immutableSignerDigest(value string) bool {
	if immutableDigestPattern.MatchString(value) {
		return true
	}
	lower := strings.ToLower(value)
	return strings.HasPrefix(lower, "${{ needs.") && strings.Contains(lower, ".outputs.") && strings.Contains(lower, "digest")
}
