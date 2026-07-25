package workflowpolicy

import "slices"

type workflowJobIdentity struct {
	workflow string
	job      string
}

type integerPermissionDelegation struct {
	identity workflowJobIdentity
	uses     string
	scopes   []permissionScope
}

var integerPermissionDelegations = []integerPermissionDelegation{
	{
		identity: workflowJobIdentity{workflow: "integer-orchestrator.yaml", job: "build-verity"},
		uses:     "./.github/workflows/build-verity.yaml",
		scopes:   []permissionScope{idTokenScope, attestationsScope},
	},
	{
		identity: workflowJobIdentity{workflow: "integer-orchestrator.yaml", job: "orchestrate"},
		uses:     integerOrchestratorReference,
		scopes:   []permissionScope{packagesScope, idTokenScope, attestationsScope},
	},
	{
		identity: workflowJobIdentity{workflow: "integer-build-image.yaml", job: "build-verity"},
		uses:     "./.github/workflows/build-verity.yaml",
		scopes:   []permissionScope{idTokenScope, attestationsScope},
	},
	{
		identity: workflowJobIdentity{workflow: "integer-build-image.yaml", job: "build"},
		uses:     integerImageWorkflowReference,
		scopes:   []permissionScope{packagesScope, idTokenScope, attestationsScope},
	},
	{
		identity: workflowJobIdentity{workflow: "integer-orchestrator-reusable.yaml", job: "build-shards"},
		uses:     integerShardWorkflowReference,
		scopes:   []permissionScope{packagesScope, idTokenScope, attestationsScope},
	},
	{
		identity: workflowJobIdentity{workflow: "integer-build-shard.yaml", job: "build"},
		uses:     integerImageWorkflowReference,
		scopes:   []permissionScope{packagesScope, idTokenScope, attestationsScope},
	},
}

var publicationPermissionDelegations = []integerPermissionDelegation{
	{
		identity: workflowJobIdentity{workflow: buildSiteWorkflowName, job: "build-verity"},
		uses:     "./.github/workflows/build-verity.yaml",
		scopes:   []permissionScope{idTokenScope, attestationsScope},
	},
	{
		identity: workflowJobIdentity{workflow: "new-issue.yaml", job: "build-verity"},
		uses:     "./.github/workflows/build-verity.yaml",
		scopes:   []permissionScope{idTokenScope, attestationsScope},
	},
	{
		identity: workflowJobIdentity{workflow: buildSiteWorkflowName, job: "integer"},
		uses:     integerOrchestratorReference,
		scopes:   []permissionScope{packagesScope, idTokenScope, attestationsScope},
	},
	{
		identity: workflowJobIdentity{workflow: buildSiteWorkflowName, job: "charts"},
		uses:     "./.github/workflows/orchestrator.yaml",
		scopes:   []permissionScope{contentsScope, packagesScope, idTokenScope, attestationsScope},
	},
	{
		identity: workflowJobIdentity{workflow: "integer-sync.yaml", job: "build-verity"},
		uses:     "./.github/workflows/build-verity.yaml",
		scopes:   []permissionScope{idTokenScope, attestationsScope},
	},
	{
		identity: workflowJobIdentity{workflow: "orchestrator.yaml", job: "build-verity"},
		uses:     "./.github/workflows/build-verity.yaml",
		scopes:   []permissionScope{idTokenScope, attestationsScope},
	},
	{
		identity: workflowJobIdentity{workflow: "patch-image.yaml", job: "build-verity"},
		uses:     "./.github/workflows/build-verity.yaml",
		scopes:   []permissionScope{idTokenScope, attestationsScope},
	},
	{
		identity: workflowJobIdentity{workflow: "publish.yaml", job: "publish"},
		uses:     "./.github/workflows/build-site.yaml",
		scopes:   []permissionScope{contentsScope, packagesScope, pagesScope, idTokenScope, attestationsScope},
	},
}

func reusableJobUsesWritePermission(workflows []workflowFile, identity workflowJobIdentity, job *workflowJob, scope permissionScope) bool {
	exact := exactReusablePermissionContract(identity, job, scope)
	if exactReusablePermissionScope(job.Uses, scope) && !exact {
		return false
	}
	if workflowName, local := localReusableWorkflowName(job.Uses); local {
		if called, exists := findWorkflow(workflows, workflowName); exists {
			return called.Workflow.On.WorkflowCall && workflowDeclaresWritePermission(&called.Workflow, scope)
		}
	}
	return exact
}

func workflowDeclaresWritePermission(called *workflow, scope permissionScope) bool {
	for _, jobName := range sortedJobNames(called.Jobs) {
		job := called.Jobs[jobName]
		if effectivePermission(called.Permissions, job.Permissions, scope) == permissionWrite {
			return true
		}
	}
	return false
}

func exactReusablePermissionContract(identity workflowJobIdentity, job *workflowJob, scope permissionScope) bool {
	for _, delegation := range integerPermissionDelegations {
		if delegation.identity == identity && delegation.uses == job.Uses && slices.Contains(delegation.scopes, scope) {
			return true
		}
	}
	for _, delegation := range publicationPermissionDelegations {
		if delegation.identity == identity && delegation.uses == job.Uses && slices.Contains(delegation.scopes, scope) {
			return true
		}
	}
	return false
}

func exactReusablePermissionScope(reference string, scope permissionScope) bool {
	for _, delegation := range integerPermissionDelegations {
		if delegation.uses == reference && slices.Contains(delegation.scopes, scope) {
			return true
		}
	}
	for _, delegation := range publicationPermissionDelegations {
		if delegation.uses == reference && slices.Contains(delegation.scopes, scope) {
			return true
		}
	}
	return false
}

func integerDelegatesWritePermission(identity workflowJobIdentity, job *workflowJob, scope permissionScope) bool {
	return exactReusablePermissionContract(identity, job, scope)
}
