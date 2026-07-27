package workflowpolicy

import (
	"slices"
	"strings"
)

var requiredProducerOutputs = []string{
	"source_sha",
	"run_id",
	"run_attempt",
	"batch_id",
	"artifact_name",
	"artifact_digest",
}

var coherentProducerOutputs = []string{
	"source_sha",
	"run_id",
	"run_attempt",
	"publication_id",
	"batch_id",
	"artifact_name",
	"artifact_digest",
}

type producerWorkflowSpec struct {
	nestedReference string
	nestedWorkflow  string
	terminal        bool
}

type downloadProducerContext struct {
	workflows []workflowFile
	file      *workflowFile
	consumer  *workflowJob
	step      *workflowStep
	stepIndex int
}

var producerWorkflowSpecs = map[string]producerWorkflowSpec{
	"integer-orchestrator-reusable.yaml": {
		nestedReference: integerShardWorkflowReference,
		nestedWorkflow:  "integer-build-shard.yaml",
	},
	"integer-build-shard.yaml": {
		nestedReference: integerImageWorkflowReference,
		nestedWorkflow:  "integer-build-image-reusable.yaml",
	},
	"integer-build-image-reusable.yaml": {terminal: true},
}

func validateDownloadProducer(context downloadProducerContext) string {
	runProducer, runOK := exactOutputReference(context.step.With["run-id"], "needs", "run_id")
	artifactProducer, artifactOK := exactOutputReference(context.step.With["name"], "needs", "artifact_name")
	if runOK && artifactOK && runProducer == artifactProducer {
		return validateNeedsDownloadProducer(context, runProducer)
	}
	runProducer, runOK = exactOutputReference(context.step.With["run-id"], "steps", "run-id")
	artifactProducer, artifactOK = exactOutputReference(context.step.With["name"], "steps", "artifact-name")
	if runOK && artifactOK && runProducer == artifactProducer && exactMetricsResolver(context, runProducer) {
		return ""
	}
	return "cross-run download must use exact run_id and artifact_name outputs from one producer"
}

func validateNeedsDownloadProducer(context downloadProducerContext, runProducer string) string {
	producer, exists := context.file.Workflow.Jobs[runProducer]
	if !exists || !containsString(context.consumer.Needs, runProducer) {
		return "cross-run download producer must exist in the consumer needs graph"
	}
	if !hasSuccessSemantics(context.consumer.If) || !hasSuccessSemantics(producer.If) {
		return "cross-run producer and consumer must retain success-only job semantics"
	}
	if producer.Uses != integerOrchestratorReference {
		return "cross-run download must use the trusted Integer orchestrator producer"
	}
	if reason := validateReusableProducer(context.workflows, "integer-orchestrator-reusable.yaml"); reason != "" {
		return reason
	}
	return ""
}

func exactMetricsResolver(context downloadProducerContext, stepID string) bool {
	for stepIndex := range context.stepIndex {
		step := &context.consumer.Steps[stepIndex]
		if step.ID != stepID {
			continue
		}
		commands := splitShellCommands(step.Run)
		if len(commands) != 1 {
			return false
		}
		invocation := parseShellInvocation(commands[0])
		if invocation.executable < 0 || invocation.workingDirectory != "" || commands[0][invocation.executable] != "./verity" {
			return false
		}
		expected := []string{
			"ci", "workflowops", "resolve-metrics-producer",
			"--run-id", "${{ inputs.run-id }}",
			"--run-attempt", "${{ inputs.run-attempt }}",
			"--source-sha", "${{ inputs.source_sha }}",
			"--artifact-name", "${{ inputs.artifact-name }}",
			"--github-output", "$GITHUB_OUTPUT",
		}
		return slices.EqualFunc(commands[0][invocation.executable+1:], expected, func(actual, wanted string) bool {
			return normalizeExpression(actual) == normalizeExpression(wanted)
		})
	}
	return false
}

func validateReusableProducer(workflows []workflowFile, workflowName string) string {
	file, exists := findWorkflow(workflows, workflowName)
	if !exists || !file.Workflow.On.WorkflowCall {
		return "trusted producer workflow must expose a workflow_call contract"
	}

	outputJob := ""
	for _, outputName := range requiredProducerOutputs {
		output, present := file.Workflow.On.WorkflowOutputs[outputName]
		jobName, exact := exactOutputReference(output.Value, "jobs", outputName)
		if !present || !exact {
			return "trusted producer workflow outputs must preserve exact correlated identity"
		}
		if outputJob == "" {
			outputJob = jobName
		} else if outputJob != jobName {
			return "trusted producer workflow outputs must come from one producer job"
		}
	}

	job, exists := file.Workflow.Jobs[outputJob]
	if !exists || !hasSuccessSemantics(job.If) {
		return "trusted producer output job must exist with success-only semantics"
	}
	spec := producerWorkflowSpecs[workflowName]
	if spec.terminal {
		return validateTerminalProducer(&job)
	}
	if job.Uses != spec.nestedReference {
		return "trusted producer chain contains an unexpected reusable workflow"
	}
	return validateReusableProducer(workflows, spec.nestedWorkflow)
}

func validateTerminalProducer(job *workflowJob) string {
	expected := map[string]string{
		"source_sha":      "${{github.sha}}",
		"run_id":          "${{github.run_id}}",
		"run_attempt":     "${{github.run_attempt}}",
		"batch_id":        "${{inputs.batch_id}}",
		"artifact_name":   normalizeExpression(approvedAPKArtifactName),
		"artifact_digest": "${{steps.upload.outputs.artifact-digest}}",
	}
	for _, outputName := range requiredProducerOutputs {
		if normalizeExpression(job.Outputs[outputName]) != expected[outputName] {
			return "terminal Integer producer outputs must bind exact immutable identity and artifact metadata"
		}
	}
	for stepIndex := range job.Steps {
		step := &job.Steps[stepIndex]
		if step.ID == "upload" && actionName(step.Uses) == "actions/upload-artifact" &&
			normalizeExpression(step.With["name"]) == normalizeExpression(approvedAPKArtifactName) &&
			strings.Contains(step.With["path"], ".apk") {
			return ""
		}
	}
	return "terminal Integer producer must expose digest output from the exact APK artifact upload"
}

func exactOutputReference(value, namespace, field string) (string, bool) {
	value = normalizeExpression(value)
	if !strings.HasPrefix(value, "${{") || !strings.HasSuffix(value, "}}") {
		return "", false
	}
	parts := strings.Split(strings.TrimSuffix(strings.TrimPrefix(value, "${{"), "}}"), ".")
	if len(parts) != 4 || parts[0] != namespace || parts[1] == "" || parts[2] != "outputs" || parts[3] != field {
		return "", false
	}
	return parts[1], true
}

func hasSuccessSemantics(value string) bool {
	switch normalizeExpression(strings.ToLower(value)) {
	case "", "success()", "${{success()}}":
		return true
	default:
		return false
	}
}

func containsString(values []string, expected string) bool {
	return slices.Contains(values, expected)
}
