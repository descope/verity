package workflowpolicy

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateSignerProvenance_accepts_exact_event_conditional_attestation(t *testing.T) {
	// Given a read-only, secret-free job that can run directly on PRs or through a reusable protected path.
	workflows := setupVerityWorkflows(
		"chart-integration.yaml",
		triggers{PullRequest: true, WorkflowCall: true},
		readOnlyPermissions(),
		nil,
	)
	setSetupVerityInput(workflows, "verify-attestation", "${{ github.event_name != 'pull_request' }}")

	// When provenance policy evaluates the event-exact attestation mode.
	violations := validateSignerProvenance(workflows)

	// Then PR activation may remain unattested while every non-PR activation verifies attestation.
	assert.NotContains(t, violationRules(violations), RuleSignerProvenance)
}

func TestValidateSignerProvenance_accepts_unattested_job_proven_PR_only(t *testing.T) {
	// Given a mixed-trigger workflow whose job gate proves it can execute only for pull requests.
	workflows := setupVerityWorkflows(
		"mixed.yaml",
		triggers{PullRequest: true, Push: pushTrigger{Present: true}},
		readOnlyPermissions(),
		nil,
	)
	setSetupVerityJobIf(workflows, "${{ github.event_name == 'pull_request' && success() }}")

	// When provenance policy evaluates its unattested exact same-run activation.
	violations := validateSignerProvenance(workflows)

	// Then the job-level capability proof keeps the PR-only lane allowed.
	assert.NotContains(t, violationRules(violations), RuleSignerProvenance)
}

func TestValidateSignerProvenance_accepts_protected_only_secret_and_signing_steps_with_conditional_attestation(t *testing.T) {
	// Given a mixed-trigger read-only job whose secret and signing capabilities are gated out of PR execution.
	workflows := setupVerityWorkflows(
		"mixed.yaml",
		triggers{PullRequest: true, WorkflowCall: true},
		readOnlyPermissions(),
		nil,
	)
	setSetupVerityInput(workflows, "verify-attestation", "${{ github.event_name != 'pull_request' }}")
	job := workflows[0].Workflow.Jobs["validate"]
	job.Steps = append(job.Steps, workflowStep{
		If:  "${{ github.event_name != 'pull_request' }}",
		Run: "cosign sign example.invalid/image@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Env: scalarMap{"TOKEN": testSecretExpression},
	})
	workflows[0].Workflow.Jobs["validate"] = job

	// When provenance policy evaluates event-scoped capabilities.
	violations := validateSignerProvenance(workflows)

	// Then the PR lane stays secret-free and non-signing while protected activation verifies attestation.
	assert.NotContains(t, violationRules(violations), RuleSignerProvenance)
}

func TestValidateSignerProvenance_accepts_exact_first_party_attestation_producer(t *testing.T) {
	// Given the first-party job that recovers a complete same-run artifact immediately before attesting it.
	workflows := []workflowFile{{Name: protectedBuildVerityWorkflowName, Workflow: workflow{
		On:          triggers{WorkflowCall: true},
		Permissions: permissions{declared: true, all: permissionNone},
		Jobs: map[string]workflowJob{
			"build": {
				If:          protectedBuildGate,
				Permissions: readOnlyPermissions(),
			},
			"attest": {
				Needs: stringList{"build"},
				Permissions: permissions{declared: true, scopes: map[permissionScope]permissionLevel{
					actionsScope: permissionRead, contentsScope: permissionRead,
					idTokenScope: permissionWrite, attestationsScope: permissionWrite,
				}},
				Steps: []workflowStep{
					setupVerityStep("false"),
					{Uses: "actions/attest-build-provenance@1111111111111111111111111111111111111111", With: scalarMap{"subject-path": "verity"}},
				},
			},
		},
	}}}

	// When signer provenance policy evaluates the attestation issuer.
	violations := validateSignerProvenance(workflows)

	// Then issuing the first attestation does not require a pre-existing attestation.
	assert.NotContains(t, violationRules(violations), RuleSignerProvenance)
}

func TestValidateSignerProvenance_rejects_attestation_producer_lookalikes(t *testing.T) {
	// Given an untrusted workflow that copies the attestation-producer step sequence.
	workflows := []workflowFile{{Name: "copy-build-verity.yaml", Workflow: workflow{
		On:          triggers{WorkflowCall: true},
		Permissions: readOnlyPermissions(),
		Jobs: map[string]workflowJob{"attest": {
			Permissions: permissions{declared: true, scopes: map[permissionScope]permissionLevel{
				contentsScope: permissionRead, idTokenScope: permissionWrite, attestationsScope: permissionWrite,
			}},
			Steps: []workflowStep{
				setupVerityStep("false"),
				{Uses: "actions/attest-build-provenance@1111111111111111111111111111111111111111", With: scalarMap{"subject-path": "verity"}},
			},
		}},
	}}}

	// When signer provenance policy evaluates the lookalike.
	violations := validateSignerProvenance(workflows)

	// Then only the exact first-party producer receives the bootstrap exception.
	assert.Contains(t, violationRules(violations), RuleSignerProvenance)
}

func TestValidateSignerProvenance_requires_complete_same_run_identity(t *testing.T) {
	for _, key := range []string{"artifact-name", "artifact-digest", "source-sha", "build-key"} {
		t.Run(key, func(t *testing.T) {
			// Given a safe PR activation missing one exact same-run coordinate.
			workflows := setupVerityWorkflows(
				"pr-test.yaml",
				triggers{PullRequest: true},
				readOnlyPermissions(),
				nil,
			)
			setSetupVerityInput(workflows, key, "")

			// When signer provenance policy evaluates the incomplete activation.
			violations := validateSignerProvenance(workflows)

			// Then an unattested path still requires every exact current-run coordinate.
			assert.Contains(t, violationRules(violations), RuleSignerProvenance)
		})
	}
}

func TestValidateSignerProvenance_rejects_inexact_attestation_conditions(t *testing.T) {
	tests := []struct {
		name       string
		condition  string
		writeScope bool
	}{
		{name: "or true", condition: "${{ github.event_name != 'pull_request' || true }}"},
		{name: "push only", condition: "${{ github.event_name == 'push' }}"},
		{name: "write capable PR", condition: "${{ github.event_name != 'pull_request' }}", writeScope: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given a mixed activation whose condition does not cover every risky execution.
			jobPermissions := readOnlyPermissions()
			if test.writeScope {
				jobPermissions.scopes[contentsScope] = permissionWrite
			}
			workflows := setupVerityWorkflows(
				"mixed.yaml",
				triggers{PullRequest: true, Push: pushTrigger{Present: true}},
				jobPermissions,
				nil,
			)
			setSetupVerityInput(workflows, "verify-attestation", test.condition)

			// When provenance policy evaluates the condition.
			violations := validateSignerProvenance(workflows)

			// Then protected or write-capable paths remain fail-closed.
			assert.Contains(t, violationRules(violations), RuleSignerProvenance)
		})
	}
}

func TestValidateSignerProvenance_rejects_PR_reachable_secret_and_signing_steps(t *testing.T) {
	tests := []struct {
		name string
		step workflowStep
	}{
		{name: "secret", step: workflowStep{Run: "true", Env: scalarMap{"TOKEN": testSecretExpression}}},
		{name: "signing", step: workflowStep{Run: "cosign sign example.invalid/image@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given a mixed-trigger job whose risky step remains reachable on pull requests.
			workflows := setupVerityWorkflows(
				"mixed.yaml",
				triggers{PullRequest: true, WorkflowCall: true},
				readOnlyPermissions(),
				nil,
			)
			setSetupVerityInput(workflows, "verify-attestation", "${{ github.event_name != 'pull_request' }}")
			job := workflows[0].Workflow.Jobs["validate"]
			job.Steps = append(job.Steps, test.step)
			workflows[0].Workflow.Jobs["validate"] = job

			// When provenance policy evaluates the under-attested PR capability.
			violations := validateSignerProvenance(workflows)

			// Then secret-bearing and signing PR paths still require attestation.
			assert.Contains(t, violationRules(violations), RuleSignerProvenance)
		})
	}
}

func setSetupVerityInput(workflows []workflowFile, key, value string) {
	job := workflows[0].Workflow.Jobs["validate"]
	job.Steps[0].With[key] = value
	workflows[0].Workflow.Jobs["validate"] = job
}

func setSetupVerityJobIf(workflows []workflowFile, value string) {
	job := workflows[0].Workflow.Jobs["validate"]
	job.If = value
	workflows[0].Workflow.Jobs["validate"] = job
}

func setupVerityStep(verify string) workflowStep {
	return workflowStep{
		Uses: "./.github/actions/setup-verity",
		With: scalarMap{
			"artifact-name":      "${{ needs.build.outputs.artifact-name }}",
			"artifact-digest":    "${{ needs.build.outputs.artifact-digest }}",
			"source-sha":         "${{ needs.build.outputs.source-sha }}",
			"build-key":          "${{ needs.build.outputs.build-key }}",
			"verify-attestation": verify,
		},
	}
}
