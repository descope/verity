package workflowpolicy

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegerOrchestrator_triggersEveryProductionInputClass(t *testing.T) {
	// Given: the production Integer orchestrator.
	workflow := readRepositoryIntegerWorkflow(t, "integer-orchestrator.yaml")

	// When: its push paths are treated as an exact set.
	paths := append([]string(nil), workflow.On.Push.Paths...)
	slices.Sort(paths)

	// Then: every recipe, pipeline, override, lock, image, Go, and workflow
	// input class can start an exact production run.
	for _, required := range []string{
		"integer.yaml", "images/**", "packages/bespoke/**", "packages/pipelines/**",
		"packages/overrides/**", "packages/upstream.lock.json", "*.go", "**/*.go",
		"go.mod", "go.sum", "mise.toml", "mise.lock", ".github/workflows/integer-orchestrator.yaml",
		".github/workflows/integer-orchestrator-reusable.yaml", ".github/workflows/integer-build-shard.yaml",
		".github/workflows/integer-build-image.yaml", ".github/workflows/integer-build-image-reusable.yaml",
	} {
		assert.Contains(t, paths, required)
	}
}

func TestIntegerWorkflows_requireExactIdentity_andExposeOnlyArtifactMetadata(t *testing.T) {
	tests := []struct {
		name    string
		inputs  []string
		outputs []string
	}{
		{
			name: "integer-orchestrator-reusable.yaml",
			inputs: []string{
				"source_sha", "verity_artifact_name", "verity_artifact_digest", "verity_build_key",
				"run_id", "run_attempt", "publication_id", "batch_id", "event",
			},
			outputs: []string{"manifest_artifact_name", "manifest_artifact_digest"},
		},
		{
			name: "integer-build-shard.yaml",
			inputs: []string{
				"source_sha", "verity_artifact_name", "verity_artifact_digest", "verity_build_key",
				"run_id", "run_attempt", "publication_id", "batch_id", "event", "mode", "shard", "entries",
				"component_count", "plan_artifact_name", "plan_artifact_digest",
			},
			outputs: []string{"manifest_artifact_name", "manifest_artifact_digest", "package_artifact_name", "package_artifact_digest"},
		},
		{
			name: "integer-build-image-reusable.yaml",
			inputs: []string{
				"source_sha", "verity_artifact_name", "verity_artifact_digest", "verity_build_key",
				"run_id", "run_attempt", "publication_id", "batch_id", "event", "mode", "shard",
				"image", "version", "type", "tags", "registry", "expected_packages", "publish_packages", "artifact_key",
			},
			outputs: []string{"artifact_name", "artifact_digest"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given: one reusable Integer producer.
			workflow := readRepositoryIntegerWorkflow(t, test.name)

			// When: its workflow_call surface is inspected.
			for _, input := range test.inputs {
				contract, ok := workflow.On.WorkflowInputs[input]
				require.True(t, ok, "missing input %s", input)
				assert.True(t, contract.Required, "input %s must be required", input)
			}

			// Then: only immutable artifact names and digests cross the reusable
			// workflow boundary.
			assert.ElementsMatch(t, test.outputs, integerWorkflowOutputNames(workflow.On.WorkflowOutputs))
			for output := range workflow.On.WorkflowOutputs {
				assert.True(t, strings.HasSuffix(output, "_name") || strings.HasSuffix(output, "_digest"), output)
			}
		})
	}
}

func TestIntegerOrchestrator_usesGoPlannedIdentityWithoutFallbacks(t *testing.T) {
	// Given: the production Integer orchestrator.
	workflow := readRepositoryIntegerWorkflowText(t, "integer-orchestrator-reusable.yaml")

	// When: identity-bearing expressions are inspected.
	identityMarkers := []string{"source_sha", "run_id", "run_attempt", "publication_id", "batch_id", "plan_artifact_name", "ref:"}

	// Then: no required identity is selected through an expression fallback,
	// and job outputs are emitted from the validated Go plan.
	for line := range strings.SplitSeq(workflow, "\n") {
		for _, marker := range identityMarkers {
			if strings.Contains(line, marker) {
				assert.NotContains(t, line, "||", line)
				break
			}
		}
	}
	assert.Contains(t, workflow, "./verity ci integer-batch outputs")
	assert.Contains(t, workflow, "publication_id: ${{ steps.plan-outputs.outputs.publication_id }}")
}

func TestIntegerWorkflows_useExactNeeds_andReadOnlyDefaults(t *testing.T) {
	// Given: the three strict Integer producer workflows.
	orchestrator := readRepositoryIntegerWorkflow(t, "integer-orchestrator-reusable.yaml")
	shard := readRepositoryIntegerWorkflow(t, "integer-build-shard.yaml")
	image := readRepositoryIntegerWorkflow(t, "integer-build-image-reusable.yaml")

	// When: dependency and default permission contracts are inspected.

	// Then: manifests wait for every child and no reusable default grants write.
	assert.ElementsMatch(t, []string{"plan", "build-shards"}, []string(orchestrator.Jobs["aggregate"].Needs))
	assert.ElementsMatch(t, []string{"build"}, []string(shard.Jobs["aggregate"].Needs))
	assert.Equal(t, permissionRead, orchestrator.Permissions.level("contents"))
	assert.Equal(t, permissionRead, shard.Permissions.level("contents"))
	assert.Equal(t, permissionRead, image.Permissions.level("contents"))
	assert.False(t, orchestrator.On.PullRequest)
	assert.False(t, shard.On.PullRequest)
	assert.False(t, image.On.PullRequest)
}

func TestIntegerBuildImage_usesGoOwnedPackageAndPublicationGates_beforeAttestation(t *testing.T) {
	// Given: the exact image producer text.
	workflow := readRepositoryIntegerWorkflowText(t, "integer-build-image-reusable.yaml")

	// When: Go-owned package testing, gates, component staging, attestations,
	// and upload are located.
	packageTest := strings.Index(workflow, "./verity ci integer-image test-packages")
	localGate := strings.Index(workflow, "Trivy publish gate (not clean, no go)")
	publish := strings.Index(workflow, "./verity ci integer-image publish")
	stage := strings.Index(workflow, "ci integer-batch component")
	attestAPKs := strings.Index(workflow, "subject-path: integer-component/packages/**/*.apk")
	attestManifest := strings.Index(workflow, "subject-path: integer-component/component.json")
	upload := strings.Index(workflow, "id: upload-component")

	// Then: native package tests and both unchanged fail-closed gates precede
	// every component attestation and upload.
	for label, position := range map[string]int{
		"package test": packageTest, "local gate": localGate, "Go publication": publish, "component stage": stage,
		"APK attestation": attestAPKs, "manifest attestation": attestManifest, "component upload": upload,
	} {
		require.GreaterOrEqual(t, position, 0, "missing %s", label)
	}
	assert.Less(t, packageTest, publish)
	assert.Less(t, localGate, stage)
	assert.Less(t, publish, stage)
	assert.Less(t, stage, attestAPKs)
	assert.Less(t, attestAPKs, upload)
	assert.Less(t, attestManifest, upload)
	assert.Contains(t, workflow[localGate:stage], "--fail-on-severity UNKNOWN,LOW,MEDIUM,HIGH,CRITICAL")
	assert.NotContains(t, workflow[localGate:stage], "continue-on-error: true")
	assert.Contains(t, workflow[publish:stage], "--source-sha")
	assert.Contains(t, workflow[publish:stage], "--run-id")
	assert.Contains(t, workflow[publish:stage], "--run-attempt")
	assert.Contains(t, workflow[publish:stage], "--publication-id")
	assert.Contains(t, workflow, "integer-component-${{ inputs.publication_id }}-${{ inputs.shard }}-${{ inputs.artifact_key }}")
	assert.Contains(t, workflow, "verity.publication_id=${PUBLICATION_ID}")
	assert.NotContains(t, workflow, "apko publish")
	assert.NotContains(t, workflow, "crane copy")
	assert.NotContains(t, workflow, "melange test \\")
}

func TestIntegerManifestProducers_useGoValidation_andAttestBeforeUpload(t *testing.T) {
	// Given: the shard and final manifest producers.
	shard := readRepositoryIntegerWorkflowText(t, "integer-build-shard.yaml")
	orchestrator := readRepositoryIntegerWorkflowText(t, "integer-orchestrator-reusable.yaml")

	// When: Go aggregation, attestation, and immutable uploads are located.
	shardAggregate := strings.Index(shard, "ci integer-batch shard")
	shardFinalize := strings.Index(shard, "ci integer-batch finalize-shard")
	shardAttest := strings.Index(shard, "subject-path: integer-shard/shard-manifest.json")
	shardUpload := strings.Index(shard, "id: upload-manifest")
	finalAggregate := strings.Index(orchestrator, "ci integer-batch aggregate")
	finalAttest := strings.Index(orchestrator, "subject-path: integer-manifest.json")
	finalUpload := strings.Index(orchestrator, "id: upload-manifest")

	// Then: repository-owned Go policy runs before each manifest attestation and upload.
	for label, position := range map[string]int{
		"shard aggregate": shardAggregate, "shard finalize": shardFinalize, "shard attest": shardAttest,
		"shard upload": shardUpload, "final aggregate": finalAggregate, "final attest": finalAttest, "final upload": finalUpload,
	} {
		require.GreaterOrEqual(t, position, 0, "missing %s", label)
	}
	assert.Less(t, shardAggregate, shardFinalize)
	assert.Less(t, shardFinalize, shardAttest)
	assert.Less(t, shardAttest, shardUpload)
	assert.Less(t, finalAggregate, finalAttest)
	assert.Less(t, finalAttest, finalUpload)
	assert.Contains(t, orchestrator, "--base-sha")
	assert.Contains(t, orchestrator, "--head-sha")
	assert.NotContains(t, orchestrator, "git diff --name-only")
	assert.NotContains(t, orchestrator, "aggregate-integer-results.sh")
}

func readRepositoryIntegerWorkflow(t *testing.T, name string) workflow {
	t.Helper()
	parsed, err := decodeWorkflow([]byte(readRepositoryIntegerWorkflowText(t, name)))
	require.NoError(t, err)
	return parsed
}

func readRepositoryIntegerWorkflowText(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", ".github", "workflows", name))
	require.NoError(t, err)
	return string(data)
}

func integerWorkflowOutputNames(outputs map[string]workflowCallOutput) []string {
	names := make([]string, 0, len(outputs))
	for name := range outputs {
		names = append(names, name)
	}
	return names
}
