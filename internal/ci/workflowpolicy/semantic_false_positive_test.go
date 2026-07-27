package workflowpolicy

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

//nolint:gosec // G101 false positive: this is a GitHub expression fixture, not credential material.
const testGitHubTokenExpression = "${{ secrets.GITHUB_TOKEN }}"

func TestValidateSignerProvenance_accepts_exact_same_run_build_for_read_only_protected_job(t *testing.T) {
	// Given a protected job that can only activate the exact read-only same-run build.
	workflows := exactSameRunBuildWorkflows()

	// When signer provenance policy evaluates the unattested activation.
	violations := validateSignerProvenance(workflows)

	// Then the exact producer graph proves artifact identity without weakening privileged jobs.
	assert.NotContains(t, violationRules(violations), RuleSignerProvenance)
}

func TestValidateSignerProvenance_rejects_inexact_same_run_build_source(t *testing.T) {
	// Given a protected read-only job whose apparent producer can fall back to caller input.
	workflows := exactSameRunBuildWorkflows()
	producer := workflows[0].Workflow.Jobs["build-verity"]
	producer.With["source_sha"] = "${{ inputs.source_sha || github.sha }}"
	workflows[0].Workflow.Jobs["build-verity"] = producer

	// When signer provenance policy evaluates the unattested activation.
	violations := validateSignerProvenance(workflows)

	// Then a github.sha lookalike cannot receive the same-run exception.
	assert.Contains(t, violationRules(violations), RuleSignerProvenance)
}

func TestValidateSignerProvenance_accepts_read_only_GITHUB_TOKEN_on_PR_lane(t *testing.T) {
	// Given conditional attestation whose PR lane has only the explicitly read-only GitHub token.
	workflows := setupVerityWorkflows(
		"chart-integration.yaml",
		triggers{PullRequest: true, WorkflowCall: true},
		permissions{declared: true, scopes: map[permissionScope]permissionLevel{
			contentsScope: permissionRead, packagesScope: permissionRead,
		}},
		nil,
	)
	setSetupVerityInput(workflows, "verify-attestation", "${{ github.event_name != 'pull_request' }}")
	job := workflows[0].Workflow.Jobs["validate"]
	job.Steps = append(job.Steps, workflowStep{
		Run: "./verity ci workflowops retry-docker-login --jitter-seconds 0",
		Env: scalarMap{"DOCKER_PASSWORD": testGitHubTokenExpression},
	})
	workflows[0].Workflow.Jobs["validate"] = job

	// When signer provenance policy evaluates event-scoped capabilities.
	violations := validateSignerProvenance(workflows)

	// Then read-only token use is governed by permissions while protected calls remain attested.
	assert.NotContains(t, violationRules(violations), RuleSignerProvenance)
}

func TestWorkflowLogicViolation_allows_ShellCheck_script_operands(t *testing.T) {
	// Given ShellCheck consumes repository shell files as data.
	run := "shellcheck .github/scripts/*.sh"

	// When Go-owned logic policy classifies the command.
	reason := workflowLogicViolation(run, "")

	// Then a linter operand is not mistaken for executing repository-owned logic.
	assert.Empty(t, reason)
}

func TestValidateCrossRunDownloads_accepts_exact_metrics_resolver_outputs(t *testing.T) {
	// Given a typed resolver that writes correlated run and artifact outputs to GITHUB_OUTPUT.
	workflows := metricsResolverWorkflows("./verity ci workflowops resolve-metrics-producer " +
		"--run-id \"${{ inputs.run-id }}\" " +
		"--run-attempt \"${{ inputs.run-attempt }}\" " +
		"--source-sha \"${{ inputs.source_sha }}\" " +
		"--artifact-name \"${{ inputs.artifact-name }}\" " +
		"--github-output \"$GITHUB_OUTPUT\"")

	// When producer identity policy validates the cross-run download.
	violations := validateCrossRunDownloads(workflows)

	// Then both outputs from the exact typed resolver form one producer identity.
	assert.NotContains(t, violationRules(violations), RuleProducerIdentity)
}

func TestValidateCrossRunDownloads_rejects_metrics_resolver_lookalike(t *testing.T) {
	// Given a command that resembles the resolver but does not write the exact output channel.
	workflows := metricsResolverWorkflows("./verity ci workflowops resolve-metrics-producer " +
		"--run-id \"${{ inputs.run-id }}\" " +
		"--run-attempt \"${{ inputs.run-attempt }}\" " +
		"--source-sha \"${{ inputs.source_sha }}\" " +
		"--artifact-name \"${{ inputs.artifact-name }}\" " +
		"--github-output /tmp/decoy")

	// When producer identity policy validates the cross-run download.
	violations := validateCrossRunDownloads(workflows)

	// Then textual resemblance cannot establish correlated producer identity.
	assert.Contains(t, violationRules(violations), RuleProducerIdentity)
}

func exactSameRunBuildWorkflows() []workflowFile {
	workflows := setupVerityWorkflows(
		"ci.yaml",
		triggers{Push: pushTrigger{Present: true}},
		readOnlyPermissions(),
		nil,
	)
	consumer := workflows[0].Workflow.Jobs["validate"]
	consumer.Needs = stringList{"build-verity"}
	consumer.Steps[0].With = scalarMap{
		"artifact-name":      "${{ needs.build-verity.outputs.artifact-name }}",
		"artifact-digest":    "${{ needs.build-verity.outputs.artifact-digest }}",
		"source-sha":         "${{ needs.build-verity.outputs.source-sha }}",
		"build-key":          "${{ needs.build-verity.outputs.build-key }}",
		"verify-attestation": "false",
	}
	workflows[0].Workflow.Jobs["validate"] = consumer
	workflows[0].Workflow.Jobs["build-verity"] = workflowJob{
		Permissions: readOnlyPermissions(),
		Uses:        "./.github/workflows/build-verity.yaml",
		With: scalarMap{
			"source_sha": "${{ github.sha }}",
		},
	}
	return workflows
}

func metricsResolverWorkflows(run string) []workflowFile {
	return []workflowFile{{
		Name: "metrics-finalize.yaml",
		Workflow: workflow{Jobs: map[string]workflowJob{
			"commit-metrics": {
				Steps: []workflowStep{
					{ID: "metadata", Run: run},
					{
						Uses: "actions/download-artifact@1111111111111111111111111111111111111111",
						With: scalarMap{
							"github-token": testGitHubTokenExpression,
							"run-id":       "${{ steps.metadata.outputs.run-id }}",
							"name":         "${{ steps.metadata.outputs.artifact-name }}",
						},
					},
				},
			},
		}},
	}}
}
