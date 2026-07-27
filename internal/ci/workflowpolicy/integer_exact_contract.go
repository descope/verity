package workflowpolicy

import (
	"slices"
	"strings"
)

type integerSurface struct {
	inputs         map[string]string
	optionalInputs map[string]string
	outputs        []string
	outputJob      string
}

var integerSurfaces = map[string]integerSurface{
	"integer-orchestrator-reusable.yaml": {
		inputs: map[string]string{
			"source_sha": workflowInputStringType, "verity_artifact_name": workflowInputStringType, "verity_artifact_digest": workflowInputStringType, "verity_build_key": workflowInputStringType,
			"run_id": workflowInputStringType, "run_attempt": workflowInputStringType, "publication_id": workflowInputStringType, "batch_id": workflowInputStringType,
			"event": workflowInputStringType,
		},
		optionalInputs: map[string]string{
			"base_sha": workflowInputStringType, "image": workflowInputStringType, "package_targets_only": workflowInputBooleanType,
		},
		outputs: []string{"manifest_artifact_name", "manifest_artifact_digest"}, outputJob: "aggregate",
	},
	"integer-build-shard.yaml": {
		inputs: map[string]string{
			"source_sha": workflowInputStringType, "verity_artifact_name": workflowInputStringType, "verity_artifact_digest": workflowInputStringType, "verity_build_key": workflowInputStringType,
			"run_id": workflowInputStringType, "run_attempt": workflowInputStringType, "publication_id": workflowInputStringType, "batch_id": workflowInputStringType,
			"event": workflowInputStringType, "mode": workflowInputStringType, "shard": workflowInputStringType, "entries": workflowInputStringType,
			"component_count": "number", "plan_artifact_name": workflowInputStringType, "plan_artifact_digest": workflowInputStringType,
		},
		outputs:   []string{"manifest_artifact_name", "manifest_artifact_digest", "package_artifact_name", "package_artifact_digest"},
		outputJob: "aggregate",
	},
	"integer-build-image-reusable.yaml": {
		inputs: map[string]string{
			"source_sha": workflowInputStringType, "verity_artifact_name": workflowInputStringType, "verity_artifact_digest": workflowInputStringType, "verity_build_key": workflowInputStringType,
			"run_id": workflowInputStringType, "run_attempt": workflowInputStringType, "publication_id": workflowInputStringType, "batch_id": workflowInputStringType,
			"event": workflowInputStringType, "mode": workflowInputStringType, "shard": workflowInputStringType, "image": workflowInputStringType, "version": workflowInputStringType, "type": workflowInputStringType,
			"tags": workflowInputStringType, "registry": workflowInputStringType, "expected_packages": workflowInputStringType, "publish_packages": workflowInputStringType,
			"artifact_key": workflowInputStringType,
		},
		optionalInputs: map[string]string{"plan_artifact_name": workflowInputStringType, "plan_artifact_digest": workflowInputStringType},
		outputs:        []string{"artifact_name", "artifact_digest"}, outputJob: "build",
	},
}

func validateIntegerExactWorkflows(workflows []workflowFile) []Violation {
	var violations []Violation
	for name, surface := range integerSurfaces {
		file, exists := findWorkflow(workflows, name)
		if !exists {
			violations = append(violations, integerViolation(name, "", "required workflow is missing"))
			continue
		}
		violations = append(violations, validateIntegerSurface(&file, surface)...)
	}
	violations = append(violations, validateIntegerNeeds(workflows)...)
	violations = append(violations, validateIntegerPropagation(workflows)...)
	violations = append(violations, validateIntegerArtifactBindings(workflows)...)
	violations = append(violations, validateIntegerArtifactChain(workflows)...)
	violations = append(violations, validateIntegerSecrets(workflows)...)
	violations = append(violations, validateIntegerStructuralTopology(workflows)...)
	return violations
}

func validateIntegerNeeds(workflows []workflowFile) []Violation {
	expected := map[string]struct {
		job    string
		needs  []string
		detail string
	}{
		"integer-orchestrator-reusable.yaml": {job: "aggregate", needs: []string{"build-shards", "plan"}, detail: "aggregate must need exactly plan and build-shards"},
		"integer-build-shard.yaml":           {job: "aggregate", needs: []string{"build"}, detail: "aggregate must need exactly build"},
	}
	var violations []Violation
	for workflowName, contract := range expected {
		file, exists := findWorkflow(workflows, workflowName)
		if !exists {
			continue
		}
		job, exists := file.Workflow.Jobs[contract.job]
		actual := append([]string(nil), job.Needs...)
		slices.Sort(actual)
		if !exists || !slices.Equal(actual, contract.needs) {
			violations = append(violations, integerViolation(workflowName, contract.job, contract.detail))
		}
	}
	return violations
}

func validateIntegerPropagation(workflows []workflowFile) []Violation {
	var violations []Violation
	orchestrator, orchestratorOK := findWorkflow(workflows, "integer-orchestrator-reusable.yaml")
	if orchestratorOK {
		job, exists := orchestrator.Workflow.Jobs["build-shards"]
		expected := map[string]string{
			"source_sha":           "${{needs.plan.outputs.source_sha}}",
			"verity_artifact_name": "${{needs.plan.outputs.verity_artifact_name}}", "verity_artifact_digest": "${{needs.plan.outputs.verity_artifact_digest}}",
			"verity_build_key": "${{needs.plan.outputs.verity_build_key}}", "run_id": "${{needs.plan.outputs.run_id}}",
			"run_attempt": "${{needs.plan.outputs.run_attempt}}", "publication_id": "${{needs.plan.outputs.publication_id}}", "batch_id": "${{needs.plan.outputs.batch_id}}",
			"event": "${{needs.plan.outputs.event}}", "mode": "${{needs.plan.outputs.mode}}",
			"shard": "${{matrix.shard}}", "entries": "${{matrix.entries}}", "component_count": "${{matrix.component_count}}",
			"plan_artifact_name":   "${{needs.plan.outputs.plan_artifact_name}}",
			"plan_artifact_digest": "${{needs.plan.outputs.plan_artifact_digest}}",
		}
		if !exists || job.Uses != integerShardWorkflowReference || !integerWithMatches(job.With, expected) {
			violations = append(violations, integerViolation(orchestrator.Name, "build-shards", "shard call must preserve exact plan identity and artifact metadata"))
		}
	}
	shard, shardOK := findWorkflow(workflows, "integer-build-shard.yaml")
	if shardOK {
		job, exists := shard.Workflow.Jobs["build"]
		expected := map[string]string{
			"source_sha":           "${{inputs.source_sha}}",
			"verity_artifact_name": "${{inputs.verity_artifact_name}}", "verity_artifact_digest": "${{inputs.verity_artifact_digest}}",
			"verity_build_key": "${{inputs.verity_build_key}}", "run_id": "${{inputs.run_id}}",
			"run_attempt": "${{inputs.run_attempt}}", "publication_id": "${{inputs.publication_id}}", "batch_id": "${{inputs.batch_id}}",
			"event": "${{inputs.event}}", "mode": "${{inputs.mode}}", "shard": "${{inputs.shard}}",
			"plan_artifact_name": "${{inputs.plan_artifact_name}}", "plan_artifact_digest": "${{inputs.plan_artifact_digest}}",
			"artifact_key": "${{matrix.artifact_key}}",
		}
		if !exists || job.Uses != integerImageWorkflowReference || !integerWithMatches(job.With, expected) {
			violations = append(violations, integerViolation(shard.Name, "build", "image call must preserve exact shard identity and artifact metadata"))
		}
	}
	return violations
}

func validateIntegerArtifactChain(workflows []workflowFile) []Violation {
	checks := []struct {
		workflow string
		job      string
		markers  []string
	}{
		{workflow: "integer-build-image-reusable.yaml", job: "build", markers: []string{
			"ci integer-batch component", "integer-component/packages/**/*.apk", "integer-component/component.json", "upload-component",
		}},
		{workflow: "integer-build-shard.yaml", job: "aggregate", markers: []string{
			"ci integer-batch shard", "upload-packages", "ci integer-batch finalize-shard", "integer-shard/shard-manifest.json", "upload-manifest",
		}},
		{workflow: "integer-orchestrator-reusable.yaml", job: "aggregate", markers: []string{
			"ci integer-batch aggregate", "integer-manifest.json", "upload-manifest",
		}},
		{workflow: "integer-orchestrator-reusable.yaml", job: "plan", markers: []string{
			"ci integer-batch plan", "integer-plan/plan.json", "upload-plan",
		}},
	}
	var violations []Violation
	for _, check := range checks {
		file, exists := findWorkflow(workflows, check.workflow)
		job, jobExists := file.Workflow.Jobs[check.job]
		if !exists || !jobExists || !integerMarkersOrdered(job.Steps, check.markers) {
			violations = append(violations, integerViolation(check.workflow, check.job, "Go validation, attestations, and immutable uploads must remain ordered"))
		}
	}
	return violations
}

func validateIntegerSecrets(workflows []workflowFile) []Violation {
	checks := []struct {
		workflow string
		job      string
	}{
		{workflow: "integer-orchestrator-reusable.yaml", job: "build-shards"},
		{workflow: "integer-build-shard.yaml", job: "build"},
	}
	var violations []Violation
	for _, check := range checks {
		file, exists := findWorkflow(workflows, check.workflow)
		job, jobExists := file.Workflow.Jobs[check.job]
		if exists && jobExists && job.Secrets.set {
			violations = append(violations, integerViolation(check.workflow, check.job, "Integer reusable workflow calls must pass no secrets when no secret is required"))
		}
	}
	return violations
}

func integerWithMatches(actual scalarMap, expected map[string]string) bool {
	for name, value := range expected {
		if normalizeExpression(actual[name]) != value {
			return false
		}
	}
	return true
}

func integerMarkersOrdered(steps []workflowStep, markers []string) bool {
	position := 0
	for index := range steps {
		step := &steps[index]
		text := step.ID + "\n" + step.Uses + "\n" + step.Run + "\n" + step.With["subject-path"]
		if strings.Contains(text, markers[position]) {
			position++
			if position == len(markers) {
				return true
			}
		}
	}
	return false
}

func integerViolation(workflowName, jobName, detail string) Violation {
	return Violation{Rule: RuleProducerIdentity, Workflow: workflowName, Job: jobName, Detail: detail}
}
