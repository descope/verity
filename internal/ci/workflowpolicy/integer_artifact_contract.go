package workflowpolicy

const integerComponentArtifactName = "integer-component-${{ inputs.publication_id }}-${{ inputs.shard }}-${{ inputs.artifact_key }}"

type integerArtifactBinding struct {
	workflow     string
	job          string
	step         string
	nameOutput   string
	digestOutput string
	artifactName string
	artifactPath string
	detail       string
}

func validateIntegerArtifactBindings(workflows []workflowFile) []Violation {
	bindings := []integerArtifactBinding{
		{
			workflow: "integer-orchestrator-reusable.yaml", job: "plan", step: "upload-plan",
			nameOutput: "plan_artifact_name", digestOutput: "plan_artifact_digest",
			artifactName: "integer-plan-${{steps.plan-outputs.outputs.publication_id}}",
			artifactPath: "integer-plan/",
			detail:       "plan artifact outputs must bind the exact immutable upload",
		},
		{
			workflow: "integer-build-image-reusable.yaml", job: "build", step: "upload-component",
			nameOutput: "artifact_name", digestOutput: "artifact_digest",
			artifactName: normalizeExpression(integerComponentArtifactName),
			artifactPath: "integer-component/",
			detail:       "component artifact outputs must bind the exact immutable upload",
		},
		{
			workflow: "integer-build-shard.yaml", job: "aggregate", step: "upload-packages",
			nameOutput: "package_artifact_name", digestOutput: "package_artifact_digest",
			artifactName: "apk-repository-${{inputs.publication_id}}-${{inputs.shard}}",
			artifactPath: "integer-shard/",
			detail:       "package artifact outputs must bind the exact immutable upload",
		},
		{
			workflow: "integer-build-shard.yaml", job: "aggregate", step: "upload-manifest",
			nameOutput: "manifest_artifact_name", digestOutput: "manifest_artifact_digest",
			artifactName: "integer-shard-manifest-${{inputs.publication_id}}-${{inputs.shard}}",
			artifactPath: "integer-shard/shard-manifest.json",
			detail:       "shard manifest outputs must bind the exact immutable upload",
		},
		{
			workflow: "integer-orchestrator-reusable.yaml", job: "aggregate", step: "upload-manifest",
			nameOutput: "manifest_artifact_name", digestOutput: "manifest_artifact_digest",
			artifactName: "integer-manifest-${{needs.plan.outputs.publication_id}}",
			artifactPath: "integer-manifest.json",
			detail:       "batch manifest outputs must bind the exact immutable upload",
		},
	}
	var violations []Violation
	for index := range bindings {
		binding := &bindings[index]
		file, fileExists := findWorkflow(workflows, binding.workflow)
		job, jobExists := file.Workflow.Jobs[binding.job]
		if !fileExists || !jobExists || !integerArtifactBindingMatches(&job, binding) {
			violations = append(violations, integerViolation(binding.workflow, binding.job, binding.detail))
		}
	}
	return violations
}

func integerArtifactBindingMatches(job *workflowJob, binding *integerArtifactBinding) bool {
	if normalizeExpression(job.Outputs[binding.nameOutput]) != binding.artifactName ||
		normalizeExpression(job.Outputs[binding.digestOutput]) != "${{steps."+binding.step+".outputs.artifact-digest}}" {
		return false
	}
	for index := range job.Steps {
		step := &job.Steps[index]
		if step.ID != binding.step || actionName(step.Uses) != "actions/upload-artifact" {
			continue
		}
		return normalizeExpression(step.With["name"]) == binding.artifactName && step.With["path"] == binding.artifactPath
	}
	return false
}
