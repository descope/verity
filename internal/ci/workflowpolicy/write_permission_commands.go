package workflowpolicy

import "slices"

type typedWritePermissionCommand struct {
	arguments []string
	scopes    []permissionScope
}

var typedWritePermissionCommands = []typedWritePermissionCommand{
	{
		arguments: []string{"ci", "repository-ops", "sync-pr"},
		scopes:    []permissionScope{contentsScope, pullRequestsScope},
	},
	{
		arguments: []string{"ci", "repository-ops", "add-standalone-image"},
		scopes:    []permissionScope{contentsScope, issuesScope, pullRequestsScope},
	},
	{
		arguments: []string{"ci", "workflowops", "archive-metrics"},
		scopes:    []permissionScope{contentsScope},
	},
	{
		arguments: []string{"ci", "workflowops", "push-reports"},
		scopes:    []permissionScope{contentsScope},
	},
	{
		arguments: []string{"nightly", "copa-orchestrator", "mirror"},
		scopes:    []permissionScope{packagesScope},
	},
	{
		arguments: []string{"ci", "repository-ops", "patch-image"},
		scopes:    []permissionScope{packagesScope},
	},
}

func jobUsesTypedWritePermission(job *workflowJob, scope permissionScope) bool {
	for stepIndex := range job.Steps {
		for _, command := range splitShellCommands(job.Steps[stepIndex].Run) {
			if commandUsesTypedWritePermission(command, scope) {
				return true
			}
		}
	}
	return false
}

func commandUsesTypedWritePermission(command []string, scope permissionScope) bool {
	invocation := parseShellInvocation(command)
	if invocation.executable < 0 || invocation.workingDirectory != "" || command[invocation.executable] != "./verity" {
		return false
	}
	arguments := command[invocation.executable+1:]
	if slices.Contains(arguments, "--help") || slices.Contains(arguments, "-h") {
		return false
	}
	for _, marker := range typedWritePermissionCommands {
		if !slices.Contains(marker.scopes, scope) || len(arguments) < len(marker.arguments) {
			continue
		}
		if slices.Equal(arguments[:len(marker.arguments)], marker.arguments) {
			return true
		}
	}
	return false
}
