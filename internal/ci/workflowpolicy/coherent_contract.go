package workflowpolicy

import (
	"strings"
)

const (
	copaOrchestratorWorkflow = "orchestrator.yaml"
	copaPatchWorkflow        = "patch-image.yaml"
	chartProducerWorkflow    = "chart-gen.yaml"
	chartIntegrationWorkflow = "chart-integration.yaml"
	privilegedChartWorkflow  = "chart-integration-privileged.yaml"

	copaPatchReference        = "./.github/workflows/patch-image.yaml"
	chartProducerReference    = "./.github/workflows/chart-gen.yaml"
	chartIntegrationReference = "./.github/workflows/chart-integration.yaml"
	privilegedChartReference  = "./.github/workflows/chart-integration-privileged.yaml"
)

var coherentWorkflowInputs = map[string][]string{
	copaOrchestratorWorkflow: {"source_sha", "run_id", "run_attempt", "publication_id", "batch_id"},
	copaPatchWorkflow:        {"source_sha", "run_id", "run_attempt", "publication_id", "batch_id"},
	chartProducerWorkflow:    coherentProducerOutputs,
	chartIntegrationWorkflow: coherentProducerOutputs,
	privilegedChartWorkflow:  coherentProducerOutputs,
}

var chartGenerationPushPaths = []string{
	"internal/chartgen/**",
	"cmd/chart_gen.go",
	".github/workflows/chart-gen.yaml",
}

func validateCoherentProducerChain(workflows []workflowFile) []Violation {
	if !hasCoherentWorkflow(workflows) {
		return nil
	}

	var violations []Violation
	for workflowName, inputs := range coherentWorkflowInputs {
		file, exists := findWorkflow(workflows, workflowName)
		if !exists {
			violations = append(violations, coherentViolation(workflowName, "", "required coherent producer workflow is missing"))
			continue
		}
		violations = append(violations, validateWorkflowCallIdentityInputs(&file, inputs)...)
		violations = append(violations, validateCoherentOutputs(&file)...)
		violations = append(violations, validateCoherentTriggers(&file)...)
		violations = append(violations, validateProducerWaits(&file)...)
	}

	for _, workflowName := range []string{chartProducerWorkflow, chartIntegrationWorkflow, privilegedChartWorkflow} {
		if file, exists := findWorkflow(workflows, workflowName); exists {
			violations = append(violations, validateExactArtifactDownload(&file)...)
		}
	}
	for _, workflowName := range []string{copaOrchestratorWorkflow, copaPatchWorkflow, chartProducerWorkflow} {
		if file, exists := findWorkflow(workflows, workflowName); exists {
			violations = append(violations, validateImmutableArtifactOutput(&file)...)
		}
	}
	for _, workflowName := range []string{chartIntegrationWorkflow, privilegedChartWorkflow} {
		if file, exists := findWorkflow(workflows, workflowName); exists {
			violations = append(violations, validateIntegrationIdentityOutput(&file)...)
			violations = append(violations, validateIntegrationSourceGate(&file)...)
		}
	}
	violations = append(violations, validateCoherentGraph(workflows)...)
	return violations
}

func hasCoherentWorkflow(workflows []workflowFile) bool {
	for workflowName := range coherentWorkflowInputs {
		if _, exists := findWorkflow(workflows, workflowName); exists {
			return true
		}
	}
	return false
}

func coherentArtifactConsumer(workflowName string) bool {
	switch workflowName {
	case chartProducerWorkflow, chartIntegrationWorkflow, privilegedChartWorkflow:
		return true
	default:
		return false
	}
}

func validateCoherentOutputs(file *workflowFile) []Violation {
	if len(file.Workflow.On.WorkflowOutputs) != len(coherentProducerOutputs) {
		return []Violation{coherentViolation(file.Name, "", "workflow_call outputs must contain only exact producer identity and artifact metadata")}
	}
	outputJob := ""
	for _, field := range coherentProducerOutputs {
		output, exists := file.Workflow.On.WorkflowOutputs[field]
		jobName, exact := exactOutputReference(output.Value, "jobs", field)
		if !exists || !exact {
			return []Violation{coherentViolation(file.Name, "", "workflow_call outputs must preserve exact correlated producer fields")}
		}
		if outputJob == "" {
			outputJob = jobName
		} else if outputJob != jobName {
			return []Violation{coherentViolation(file.Name, "", "workflow_call outputs must come from one successful producer job")}
		}
	}
	if _, exists := file.Workflow.Jobs[outputJob]; !exists {
		return []Violation{coherentViolation(file.Name, outputJob, "workflow_call output producer job is missing")}
	}
	return nil
}

func validateCoherentTriggers(file *workflowFile) []Violation {
	if file.Name == copaPatchWorkflow {
		return nil
	}
	if file.Name == copaOrchestratorWorkflow {
		if !file.Workflow.On.Push.Present {
			return nil
		}
		for _, path := range chartGenerationPushPaths {
			if !containsString(file.Workflow.On.Push.Paths, path) {
				return []Violation{coherentViolation(file.Name, "", "orchestrator push paths must preserve every chart generation entry point")}
			}
		}
		return nil
	}
	if file.Workflow.On.Schedule || file.Workflow.On.WorkflowRun {
		return []Violation{coherentViolation(file.Name, "", "reusable chart producers must not correlate through schedules or workflow_run")}
	}
	return nil
}

func validateProducerWaits(file *workflowFile) []Violation {
	for _, jobName := range sortedJobNames(file.Workflow.Jobs) {
		steps := file.Workflow.Jobs[jobName].Steps
		for index := range steps {
			step := &steps[index]
			run := strings.ToLower(step.Run)
			if strings.Contains(run, "wait-for-workflows") || strings.Contains(run, "wait_for_workflows") {
				return []Violation{coherentViolation(file.Name, jobName, "producer polling and workflow waits are forbidden")}
			}
		}
	}
	return nil
}

func validateImmutableArtifactOutput(file *workflowFile) []Violation {
	for _, jobName := range sortedJobNames(file.Workflow.Jobs) {
		job := file.Workflow.Jobs[jobName]
		artifactName := normalizeExpression(job.Outputs["artifact_name"])
		if !strings.Contains(artifactName, "inputs.publication_id") {
			continue
		}
		stepName, exact := exactOutputReference(job.Outputs["artifact_digest"], "steps", "artifact-digest")
		if !exact {
			continue
		}
		for index := range job.Steps {
			step := &job.Steps[index]
			if step.ID == stepName && actionName(step.Uses) == "actions/upload-artifact" &&
				normalizeExpression(step.With["name"]) == normalizeExpression(job.Outputs["artifact_name"]) {
				if requiresProducerManifest(file.Name) && !validProducerManifestUpload(&job, index) {
					continue
				}
				return nil
			}
		}
	}
	return []Violation{coherentViolation(file.Name, "", "artifact_name must include publication_id and artifact_digest must bind the exact immutable upload")}
}

func requiresProducerManifest(workflowName string) bool {
	return workflowName == copaOrchestratorWorkflow || workflowName == chartProducerWorkflow
}

func validProducerManifestUpload(job *workflowJob, uploadIndex int) bool {
	if uploadIndex == 0 || !strings.Contains(job.Steps[uploadIndex].With["path"], "producer-manifest.json") {
		return false
	}
	step := job.Steps[uploadIndex-1]
	if strings.TrimSpace(step.Run) != "./verity ci artifact-provenance write-manifest" {
		return false
	}
	expected := map[string]string{
		"PROVENANCE_REPOSITORY":     "${{github.repository}}",
		"PROVENANCE_RUN_ID":         job.Outputs["run_id"],
		"PROVENANCE_RUN_ATTEMPT":    job.Outputs["run_attempt"],
		"PROVENANCE_SOURCE_SHA":     job.Outputs["source_sha"],
		"PROVENANCE_PUBLICATION_ID": job.Outputs["publication_id"],
		"PROVENANCE_ARTIFACT_NAME":  job.Outputs["artifact_name"],
		"PROVENANCE_MANIFEST":       "producer-manifest.json",
	}
	for name, value := range expected {
		if normalizeExpression(step.Env[name]) != normalizeExpression(value) {
			return false
		}
	}
	return len(step.Env) == len(expected)
}

func coherentViolation(workflowName, jobName, detail string) Violation {
	return Violation{Rule: RuleProducerIdentity, Workflow: workflowName, Job: jobName, Detail: detail}
}
