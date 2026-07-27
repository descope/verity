package workflowpolicy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateIntegerContract_acceptsRepositoryExactProducers(t *testing.T) {
	// Given: the three repository Integer producer workflows.
	workflows := repositoryIntegerWorkflowFiles(t)

	// When: the Go-owned Integer policy evaluates their complete contract.
	violations := validateIntegerContract(workflows)

	// Then: exact identity, graph, artifact, and trigger policy all pass.
	assert.Empty(t, violations)
}

func TestValidateIntegerContract_rejectsMissingExactOutput_andWeakenedNeeds(t *testing.T) {
	// Given: valid producers with one missing final digest and a manifest that
	// no longer waits for every shard.
	workflows := repositoryIntegerWorkflowFiles(t)
	orchestrator := &workflows[0].Workflow
	delete(orchestrator.On.WorkflowOutputs, "manifest_artifact_digest")
	aggregate := orchestrator.Jobs["aggregate"]
	aggregate.Needs = stringList{"plan"}
	orchestrator.Jobs["aggregate"] = aggregate

	// When: the weakened contract is evaluated.
	violations := validateIntegerContract(workflows)

	// Then: producer identity policy reports both exactness failures.
	require.NotEmpty(t, violations)
	details := make([]string, 0, len(violations))
	for _, violation := range violations {
		details = append(details, violation.Detail)
	}
	assert.Contains(t, details, "workflow_call outputs must contain only exact artifact names and digests")
	assert.Contains(t, details, "aggregate must need exactly plan and build-shards")
}

func TestValidateIntegerContract_rejectsIdentityWorkflowOutput(t *testing.T) {
	// Given: a reusable image producer exposes publication identity as an
	// output instead of keeping it inside its attested artifact.
	workflows := repositoryIntegerWorkflowFiles(t)
	image := &workflows[2].Workflow
	image.On.WorkflowOutputs["publication_id"] = workflowCallOutput{Value: "${{ jobs.build.outputs.publication_id }}"}

	// When: the exact reusable surface is validated.
	violations := validateIntegerContract(workflows)

	// Then: only artifact names and digests may cross the call boundary.
	details := make([]string, 0, len(violations))
	for _, violation := range violations {
		details = append(details, violation.Detail)
	}
	assert.Contains(t, details, "workflow_call outputs must contain only exact artifact names and digests")
}

func TestIntegerReusableProducerPermissions_recognizeChildAttestationUse(t *testing.T) {
	// Given: the orchestrator and shard reusable producer jobs.
	workflows := repositoryIntegerWorkflowFiles(t)
	orchestrator := workflows[0].Workflow.Jobs["build-shards"]
	shard := workflows[1].Workflow.Jobs["build"]

	// When: least-privilege policy maps the delegated child capability.

	// Then: both callers are recognized as using their attestation grant.
	assert.True(t, integerDelegatesWritePermission(
		workflowJobIdentity{workflow: "integer-orchestrator-reusable.yaml", job: "build-shards"},
		&orchestrator,
		attestationsScope,
	))
	assert.True(t, integerDelegatesWritePermission(
		workflowJobIdentity{workflow: "integer-build-shard.yaml", job: "build"},
		&shard,
		attestationsScope,
	))
}

func TestValidateIntegerContract_rejectsStaleComponentArtifactBinding(t *testing.T) {
	// Given: a terminal image producer whose output name no longer matches its
	// immutable component upload.
	workflows := repositoryIntegerWorkflowFiles(t)
	image := &workflows[2].Workflow
	build := image.Jobs["build"]
	build.Outputs["artifact_name"] = "integer-component-latest"
	image.Jobs["build"] = build

	// When: exact producer policy evaluates the stale binding.
	violations := validateIntegerContract(workflows)

	// Then: publication fails closed on producer identity.
	require.NotEmpty(t, violations)
	details := make([]string, 0, len(violations))
	for _, violation := range violations {
		details = append(details, violation.Detail)
	}
	assert.Contains(t, details, "component artifact outputs must bind the exact immutable upload")
}

func TestValidateIntegerContract_rejectsBatchID_asPublicationArtifactIdentity(t *testing.T) {
	// Given: the component artifact is consistently renamed to use batch_id.
	workflows := repositoryIntegerWorkflowFiles(t)
	image := &workflows[2].Workflow
	build := image.Jobs["build"]
	build.Outputs["artifact_name"] = "integer-component-${{ inputs.batch_id }}-${{ inputs.shard }}-${{ inputs.artifact_key }}"
	for index := range build.Steps {
		step := &build.Steps[index]
		if step.ID == "upload-component" {
			step.With["name"] = build.Outputs["artifact_name"]
		}
	}
	image.Jobs["build"] = build

	// When: the exact artifact contract is evaluated.
	violations := validateIntegerContract(workflows)

	// Then: batch identity cannot substitute for publication identity.
	require.NotEmpty(t, violations)
	details := make([]string, 0, len(violations))
	for _, violation := range violations {
		details = append(details, violation.Detail)
	}
	assert.Contains(t, details, "component artifact outputs must bind the exact immutable upload")
}

func TestValidateIntegerContract_rejectsBroadSecretsInheritance(t *testing.T) {
	// Given: each Integer reusable workflow caller is changed to inherit all caller secrets.
	workflows := repositoryIntegerWorkflowFiles(t)
	for _, jobName := range []struct {
		workflow int
		job      string
	}{{workflow: 0, job: "build-shards"}, {workflow: 1, job: "build"}} {
		caller := workflows[jobName.workflow].Workflow.Jobs[jobName.job]
		caller.Secrets.set = true
		caller.Secrets.inherit = true
		workflows[jobName.workflow].Workflow.Jobs[jobName.job] = caller
	}

	// When: the exact Integer contract is evaluated.
	violations := validateIntegerContract(workflows)

	// Then: broad inheritance is rejected at both reusable call boundaries.
	details := make([]string, 0, len(violations))
	for _, violation := range violations {
		details = append(details, violation.Detail)
	}
	assert.Contains(t, details, "Integer reusable workflow calls must pass no secrets when no secret is required")
}

func TestRepositoryIntegerWorkflows_passGlobalSecurityPolicies(t *testing.T) {
	// Given: the exact repository Integer producer workflows.
	workflows := repositoryIntegerWorkflowFiles(t)

	// When: shared least-privilege, pinning, Go-ownership, and zero-CVE policy
	// are evaluated without unrelated workflow fixtures.
	violations := make([]Violation, 0, len(workflows)*4)
	violations = append(violations, validatePermissions(workflows)...)
	violations = append(violations, validatePinnedReferences(workflows)...)
	violations = append(violations, validateGoOwnedLogic(workflows)...)
	violations = append(violations, validateZeroCVEOrdering(workflows)...)

	// Then: no producer bypass or shell-owned internal policy remains.
	assert.Empty(t, violations)
}

func repositoryIntegerWorkflowFiles(t *testing.T) []workflowFile {
	t.Helper()
	names := []string{
		"integer-orchestrator-reusable.yaml",
		"integer-build-shard.yaml",
		"integer-build-image-reusable.yaml",
		"integer-orchestrator.yaml",
		"integer-build-image.yaml",
	}
	workflows := make([]workflowFile, 0, len(names))
	for _, name := range names {
		workflows = append(workflows, workflowFile{Name: name, Workflow: readRepositoryIntegerWorkflow(t, name)})
	}
	return workflows
}
