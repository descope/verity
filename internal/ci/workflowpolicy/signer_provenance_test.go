package workflowpolicy

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

//nolint:gosec // G101 false positive: this is a GitHub secret reference fixture, not credential material.
const testSecretExpression = "${{ secrets.TEST_TOKEN }}"

func TestValidateSignerProvenance_allows_unattested_same_run_binary_only_for_read_only_secret_free_PR_job(t *testing.T) {
	// Given a pull-request-only job with explicit read-only permissions and no secret context.
	workflows := setupVerityWorkflows(
		"pr-test.yaml",
		triggers{PullRequest: true},
		permissions{declared: true, scopes: map[permissionScope]permissionLevel{contentsScope: permissionRead}},
		nil,
	)

	// When signer provenance policy evaluates an exact same-run artifact without protected attestation.
	violations := validateSignerProvenance(workflows)

	// Then untrusted PR validation remains allowed in the least-privileged lane.
	assert.NotContains(t, violationRules(violations), RuleSignerProvenance)
}

func TestValidateSignerProvenance_requires_attestation_for_risky_jobs(t *testing.T) {
	tests := []struct {
		name        string
		workflow    string
		triggers    triggers
		permissions permissions
		env         scalarMap
	}{
		{
			name:        "protected push",
			workflow:    "build.yaml",
			triggers:    triggers{Push: pushTrigger{Present: true}},
			permissions: readOnlyPermissions(),
		},
		{
			name:        "mixed PR and protected dispatch",
			workflow:    "build.yaml",
			triggers:    triggers{PullRequest: true, WorkflowDispatch: true},
			permissions: readOnlyPermissions(),
		},
		{
			name:     "write-capable PR",
			workflow: "pr-test.yaml",
			triggers: triggers{PullRequest: true},
			permissions: permissions{declared: true, scopes: map[permissionScope]permissionLevel{
				contentsScope: permissionWrite,
			}},
		},
		{
			name:        "secret-bearing PR",
			workflow:    "pr-test.yaml",
			triggers:    triggers{PullRequest: true},
			permissions: readOnlyPermissions(),
			env:         scalarMap{"TOKEN": testSecretExpression},
		},
		{
			name:        "signing PR",
			workflow:    buildSiteWorkflowName,
			triggers:    triggers{PullRequest: true},
			permissions: readOnlyPermissions(),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given one protected, write-capable, secret-bearing, or signing activation.
			workflows := setupVerityWorkflows(test.workflow, test.triggers, test.permissions, test.env)

			// When provenance policy evaluates an unattested setup-verity activation.
			violations := validateSignerProvenance(workflows)

			// Then the risky activation remains fail-closed.
			assert.Contains(t, violationRules(violations), RuleSignerProvenance)
		})
	}
}

func TestValidateSignerProvenance_requires_digest_for_read_only_secret_free_PR_job(t *testing.T) {
	// Given an otherwise least-privileged PR activation with no artifact digest.
	workflows := setupVerityWorkflows(
		"pr-test.yaml",
		triggers{PullRequest: true},
		readOnlyPermissions(),
		nil,
	)
	job := workflows[0].Workflow.Jobs["validate"]
	job.Steps[0].With["artifact-digest"] = ""
	workflows[0].Workflow.Jobs["validate"] = job

	// When signer provenance policy evaluates the activation.
	violations := validateSignerProvenance(workflows)

	// Then exact same-run artifact identity is still mandatory.
	assert.Contains(t, violationRules(violations), RuleSignerProvenance)
}

func setupVerityWorkflows(name string, events triggers, jobPermissions permissions, env scalarMap) []workflowFile {
	jobName := "validate"
	if name == buildSiteWorkflowName {
		jobName = buildSiteSignerJob
	}
	return []workflowFile{{
		Name: name,
		Workflow: workflow{
			On:          events,
			Permissions: readOnlyPermissions(),
			Jobs: map[string]workflowJob{
				jobName: {
					Permissions: jobPermissions,
					Steps: []workflowStep{{
						Uses: "./.github/actions/setup-verity",
						With: scalarMap{
							"artifact-name":      "${{ needs.build.outputs.artifact-name }}",
							"artifact-digest":    "${{ needs.build.outputs.artifact-digest }}",
							"source-sha":         "${{ needs.build.outputs.source-sha }}",
							"build-key":          "${{ needs.build.outputs.build-key }}",
							"verify-attestation": "false",
						},
						Env: env,
					}},
				},
			},
		},
	}}
}

func readOnlyPermissions() permissions {
	return permissions{declared: true, scopes: map[permissionScope]permissionLevel{contentsScope: permissionRead}}
}
