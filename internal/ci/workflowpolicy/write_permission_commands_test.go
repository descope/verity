package workflowpolicy

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestJobUsesWritePermission_recognizes_exact_typed_Verity_commands(t *testing.T) {
	tests := []struct {
		name  string
		run   string
		scope permissionScope
	}{
		{name: "sync PR consumes contents", run: "./verity ci repository-ops sync-pr --base main", scope: contentsScope},
		{name: "sync PR consumes pull requests", run: "./verity ci repository-ops sync-pr --base main", scope: pullRequestsScope},
		{name: "standalone image consumes contents", run: "./verity ci repository-ops add-standalone-image --name rclone", scope: contentsScope},
		{name: "standalone image consumes issues", run: "./verity ci repository-ops add-standalone-image --name rclone", scope: issuesScope},
		{name: "standalone image consumes pull requests", run: "./verity ci repository-ops add-standalone-image --name rclone", scope: pullRequestsScope},
		{name: "metrics archive consumes contents", run: "./verity ci workflowops archive-metrics ./metrics 1 1 2026-01-01T00:00:00Z", scope: contentsScope},
		{name: "report publisher consumes contents", run: "./verity ci workflowops push-reports reports/pre.json pre.json", scope: contentsScope},
		{name: "COPA mirror consumes packages", run: "./verity nightly copa-orchestrator mirror --source source --target target", scope: packagesScope},
		{name: "COPA patch consumes packages", run: "./verity ci repository-ops patch-image --source source --staging-registry target", scope: packagesScope},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given a job invoking one exact typed Verity command.
			job := workflowJob{Steps: []workflowStep{{Run: test.run}}}

			// When least-privilege policy checks whether the write is consumed.
			used := jobUsesWritePermission(&job, test.scope)

			// Then the exact command proves the scoped write is used.
			assert.True(t, used)
		})
	}
}

func TestJobUsesWritePermission_rejects_typed_command_lookalikes(t *testing.T) {
	tests := []struct {
		name  string
		run   string
		scope permissionScope
	}{
		{name: "echoed command", run: "echo ./verity ci repository-ops sync-pr", scope: contentsScope},
		{name: "subcommand suffix", run: "./verity ci repository-ops sync-pr-preview", scope: pullRequestsScope},
		{name: "different binary", run: "./tools/verity ci workflowops archive-metrics", scope: contentsScope},
		{name: "different working directory", run: "cd tools && ./verity ci workflowops push-reports", scope: contentsScope},
		{name: "COPA mirror suffix", run: "./verity nightly copa-orchestrator mirror-image", scope: packagesScope},
		{name: "COPA patch suffix", run: "./verity ci repository-ops patch-image-report", scope: packagesScope},
		{name: "help only", run: "./verity ci repository-ops sync-pr --help", scope: pullRequestsScope},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given command text that resembles, but does not invoke, an approved typed command.
			job := workflowJob{Steps: []workflowStep{{Run: test.run}}}

			// When least-privilege policy checks the claimed write.
			used := jobUsesWritePermission(&job, test.scope)

			// Then text resemblance cannot grant the capability.
			assert.False(t, used)
		})
	}
}

func TestValidatePermissions_accepts_exact_new_issue_build_Verity_delegation(t *testing.T) {
	// Given the exact new-issue caller and local build-verity workflow capability.
	workflows := []workflowFile{
		{
			Name: "new-issue.yaml",
			Workflow: workflow{
				Permissions: permissions{declared: true, scopes: map[permissionScope]permissionLevel{}},
				Jobs: map[string]workflowJob{
					"build-verity": {
						Uses: protectedBuildVerityWorkflowReference,
						Permissions: permissions{declared: true, scopes: map[permissionScope]permissionLevel{
							idTokenScope: permissionWrite, attestationsScope: permissionWrite,
						}},
					},
				},
			},
		},
		{
			Name: protectedBuildVerityWorkflowName,
			Workflow: workflow{
				On:          triggers{WorkflowCall: true},
				Permissions: readOnlyPermissions(),
				Jobs: map[string]workflowJob{
					"attest": {
						Permissions: permissions{declared: true, scopes: map[permissionScope]permissionLevel{
							contentsScope: permissionRead, idTokenScope: permissionWrite, attestationsScope: permissionWrite,
						}},
						Steps: []workflowStep{{Uses: "actions/attest-build-provenance@1111111111111111111111111111111111111111"}},
					},
				},
			},
		},
	}

	// When least-privilege policy evaluates the exact caller/callee pair.
	violations := validatePermissions(workflows)

	// Then the delegated attestation writes are recognized as consumed.
	assert.NotContains(t, violationRules(violations), RuleLeastPrivilege)
}
