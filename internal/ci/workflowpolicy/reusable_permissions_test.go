package workflowpolicy

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidatePermissions_accepts_local_reusable_caller_cap_declared_by_called_workflow(t *testing.T) {
	// Given a local reusable caller whose write cap is declared by the called workflow.
	workflows := reusablePermissionWorkflows(packagesScope)

	// When least-privilege policy evaluates the typed caller and callee contracts.
	violations := validatePermissions(workflows)

	// Then the caller cap is recognized without relying on caller step text.
	assert.NotContains(t, violationRules(violations), RuleLeastPrivilege)
}

func TestValidatePermissions_rejects_local_reusable_caller_cap_absent_from_called_workflow(t *testing.T) {
	// Given a local reusable caller with a write cap the called workflow never declares.
	workflows := reusablePermissionWorkflows(contentsScope)

	// When least-privilege policy evaluates the typed caller and callee contracts.
	violations := validatePermissions(workflows)

	// Then the excess cap remains fail-closed even though the called step contains a package-write marker.
	assert.Contains(t, violationRules(violations), RuleLeastPrivilege)
}

func TestValidatePermissions_rejects_local_reusable_caller_marker_spoof(t *testing.T) {
	// Given a local call whose filename matches a legacy write marker but whose typed contract does not.
	workflows := reusablePermissionWorkflows(idTokenScope)
	caller := workflows[0].Workflow.Jobs["call"]
	caller.Uses = "./.github/workflows/patch-image.yaml"
	workflows[0].Workflow.Jobs["call"] = caller
	workflows[1].Name = "patch-image.yaml"

	// When least-privilege policy evaluates the reusable call.
	violations := validatePermissions(workflows)

	// Then filename text cannot spoof a declared write need.
	assert.Contains(t, violationRules(violations), RuleLeastPrivilege)
}

func reusablePermissionWorkflows(callerScope permissionScope) []workflowFile {
	explicitNone := permissions{declared: true, scopes: map[permissionScope]permissionLevel{}}
	return []workflowFile{
		{
			Name: "caller.yaml",
			Workflow: workflow{
				Permissions: explicitNone,
				Jobs: map[string]workflowJob{
					"call": {
						Permissions: permissions{declared: true, scopes: map[permissionScope]permissionLevel{callerScope: permissionWrite}},
						Uses:        "./.github/workflows/called.yaml",
					},
				},
			},
		},
		{
			Name: "called.yaml",
			Workflow: workflow{
				On:          triggers{WorkflowCall: true},
				Permissions: explicitNone,
				Jobs: map[string]workflowJob{
					"publish": {
						Permissions: permissions{declared: true, scopes: map[permissionScope]permissionLevel{packagesScope: permissionWrite}},
						Steps:       []workflowStep{{Run: "./verity ci integer-image publish"}},
					},
				},
			},
		},
	}
}
