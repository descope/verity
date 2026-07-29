package workflowpolicy

import (
	"fmt"
	"strings"
)

func validateExactArtifactDownload(file *workflowFile) []Violation {
	downloadJobs := artifactDownloadJobs(file)
	if len(downloadJobs) == 0 {
		return []Violation{coherentViolation(file.Name, "", "producer manifest download must use exact artifact_name, run_id, and artifact_digest contract")}
	}

	var violations []Violation
	for _, jobName := range downloadJobs {
		steps := file.Workflow.Jobs[jobName].Steps
		for index := range steps {
			step := &steps[index]
			if actionName(step.Uses) != "actions/download-artifact" {
				continue
			}
			if normalizeExpression(step.With["name"]) != "${{inputs.artifact_name}}" ||
				normalizeExpression(step.With["run-id"]) != "${{inputs.run_id}}" ||
				normalizeExpression(step.With["repository"]) != "${{github.repository}}" ||
				normalizeExpression(step.With["github-token"]) != "${{secrets.GITHUB_TOKEN}}" {
				violations = append(violations, coherentViolation(file.Name, jobName, "every producer manifest download must pin repository, artifact_name, and run_id"))
				continue
			}
			if index+1 >= len(steps) || !validArtifactVerificationStep(step, &steps[index+1]) {
				violations = append(violations, coherentViolation(file.Name, jobName, "every producer manifest download must be immediately verified against run attempt, source, publication, name, digest, and manifest"))
			}
		}
	}
	return violations
}

func validArtifactVerificationStep(download, verification *workflowStep) bool {
	if strings.TrimSpace(verification.Run) != "./verity ci artifact-provenance verify-download" ||
		normalizeExpression(verification.If) != normalizeExpression(download.If) {
		return false
	}
	expected := map[string]string{
		"PROVENANCE_API_AUTH":        "${{secrets.GITHUB_TOKEN}}",
		"PROVENANCE_REPOSITORY":      "${{github.repository}}",
		"PROVENANCE_RUN_ID":          "${{inputs.run_id}}",
		"PROVENANCE_RUN_ATTEMPT":     "${{inputs.run_attempt}}",
		"PROVENANCE_SOURCE_SHA":      "${{inputs.source_sha}}",
		"PROVENANCE_PUBLICATION_ID":  "${{inputs.publication_id}}",
		"PROVENANCE_ARTIFACT_NAME":   "${{inputs.artifact_name}}",
		"PROVENANCE_ARTIFACT_DIGEST": "${{inputs.artifact_digest}}",
		"PROVENANCE_MANIFEST":        strings.TrimSuffix(download.With["path"], "/") + "/producer-manifest.json",
	}
	for name, value := range expected {
		if normalizeExpression(verification.Env[name]) != normalizeExpression(value) {
			return false
		}
	}
	return len(verification.Env) == len(expected)
}

func validateIntegrationIdentityOutput(file *workflowFile) []Violation {
	outputName, outputJob, found := integrationOutputJob(file)
	if !found {
		return []Violation{coherentViolation(file.Name, "", "integration result must preserve the exact successful chart producer identity")}
	}
	if len(outputJob.Needs) == 0 || !gatesEquivalent(outputJob.If, expectedIntegrationResultGate(file.Name, outputJob.Needs)) {
		return []Violation{coherentViolation(file.Name, outputName, "integration result must require successful consumer jobs")}
	}

	downloadJobs := artifactDownloadJobs(file)
	for _, need := range outputJob.Needs {
		if !containsString(downloadJobs, need) {
			return []Violation{coherentViolation(file.Name, need, "every integration consumer must download the exact producer artifact")}
		}
	}
	for _, downloadJob := range downloadJobs {
		if !containsString(outputJob.Needs, downloadJob) {
			return []Violation{coherentViolation(file.Name, outputName, "integration result must directly need every artifact consumer")}
		}
	}
	return validateChartResultAggregation(file, outputName, &outputJob)
}

func validateChartResultAggregation(file *workflowFile, jobName string, job *workflowJob) []Violation {
	if file.Name != chartIntegrationWorkflow && file.Name != privilegedChartWorkflow {
		return nil
	}
	expectedNeeds := []string{"discover-charts", "chart-test"}
	expectedCommand := "./verity ci workflowops aggregate-chart-results --result discover-charts=${{ needs.discover-charts.result }} --result chart-test=${{ needs.chart-test.result }}"
	if file.Name == privilegedChartWorkflow {
		expectedNeeds = []string{"chart-test"}
		expectedCommand = "./verity ci workflowops aggregate-chart-results --profile privileged --result chart-test=${{ needs.chart-test.result }}"
	}
	if len(job.Needs) != len(expectedNeeds) {
		return []Violation{coherentViolation(file.Name, jobName, "chart result aggregation must declare the exact expected result set")}
	}
	for _, need := range expectedNeeds {
		if !containsString(job.Needs, need) {
			return []Violation{coherentViolation(file.Name, jobName, "chart result aggregation must declare the exact expected result set")}
		}
	}
	var aggregate *workflowStep
	for index := range job.Steps {
		if job.Steps[index].ID == "aggregate" {
			aggregate = &job.Steps[index]
			break
		}
	}
	if aggregate == nil || normalizeCommand(aggregate.Run) != normalizeCommand(expectedCommand) || workflowLogicViolation(aggregate.Run, aggregate.Shell) != "" {
		return []Violation{coherentViolation(file.Name, jobName, "chart result aggregation must invoke the typed Go result command without inline shell policy")}
	}
	expectedEnvironment := map[string]string{
		"CHART_SOURCE_SHA":      "${{inputs.source_sha}}",
		"CHART_RUN_ID":          "${{inputs.run_id}}",
		"CHART_RUN_ATTEMPT":     "${{inputs.run_attempt}}",
		"CHART_PUBLICATION_ID":  "${{inputs.publication_id}}",
		"CHART_BATCH_ID":        "${{inputs.batch_id}}",
		"CHART_ARTIFACT_NAME":   "${{inputs.artifact_name}}",
		"CHART_ARTIFACT_DIGEST": "${{inputs.artifact_digest}}",
	}
	if len(aggregate.Env) != len(expectedEnvironment) {
		return []Violation{coherentViolation(file.Name, jobName, "chart result aggregation must receive only the exact producer identity")}
	}
	for name, expected := range expectedEnvironment {
		if normalizeExpression(aggregate.Env[name]) != expected {
			return []Violation{coherentViolation(file.Name, jobName, "chart result aggregation must receive only the exact producer identity")}
		}
	}
	return nil
}

func normalizeCommand(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func validateIntegrationSourceGate(file *workflowFile) []Violation {
	_, outputJob, found := integrationOutputJob(file)
	if !found {
		return nil
	}
	for _, consumerName := range outputJob.Needs {
		consumer := file.Workflow.Jobs[consumerName]
		if !gatesEquivalent(consumer.If, expectedIntegrationConsumerGate(file, &consumer)) {
			return []Violation{coherentViolation(file.Name, consumerName, "chart integration consumer must fail closed unless producer source_sha matches github.sha")}
		}
	}
	return nil
}

func expectedIntegrationResultGate(workflowName string, needs []string) string {
	parts := make([]string, 0, len(needs))
	for _, need := range needs {
		if workflowName == chartIntegrationWorkflow && need == "chart-test" {
			parts = append(parts, fmt.Sprintf("(needs.%[1]s.result == 'success' || needs.%[1]s.result == 'skipped')", need))
			continue
		}
		parts = append(parts, fmt.Sprintf("needs.%s.result == 'success'", need))
	}
	return strings.Join(parts, " && ")
}

func expectedIntegrationConsumerGate(file *workflowFile, consumer *workflowJob) string {
	sourceGate := "inputs.source_sha == github.sha"
	if !file.Workflow.On.PullRequest {
		return appendRunsOnTrustGate(sourceGate, consumer)
	}
	pullRequestGate := "github.event_name == 'pull_request' || " + sourceGate
	if len(consumer.Needs) == 0 {
		return appendRunsOnTrustGate(pullRequestGate, consumer)
	}
	gate := fmt.Sprintf("(%s) && needs.%s.outputs.matrix != '[]'", pullRequestGate, consumer.Needs[0])
	return appendRunsOnTrustGate(gate, consumer)
}

func appendRunsOnTrustGate(gate string, job *workflowJob) string {
	if len(job.RunsOn) != 1 || !strings.HasPrefix(job.RunsOn[0], "runs-on=") {
		return gate
	}
	return fmt.Sprintf("(%s) && (%s)", gate, runsOnTrustedExecution)
}

func integrationOutputJob(file *workflowFile) (string, workflowJob, bool) {
	for _, jobName := range sortedJobNames(file.Workflow.Jobs) {
		job := file.Workflow.Jobs[jobName]
		matches := true
		for _, field := range coherentProducerOutputs {
			expected := fmt.Sprintf("${{inputs.%s}}", field)
			if file.Name == chartIntegrationWorkflow || file.Name == privilegedChartWorkflow {
				expected = fmt.Sprintf("${{steps.aggregate.outputs.%s}}", field)
			}
			if normalizeExpression(job.Outputs[field]) != expected {
				matches = false
				break
			}
		}
		if matches {
			return jobName, job, true
		}
	}
	return "", workflowJob{}, false
}

func artifactDownloadJobs(file *workflowFile) []string {
	jobs := make([]string, 0)
	for _, jobName := range sortedJobNames(file.Workflow.Jobs) {
		steps := file.Workflow.Jobs[jobName].Steps
		for index := range steps {
			step := &steps[index]
			if actionName(step.Uses) == "actions/download-artifact" {
				jobs = append(jobs, jobName)
				break
			}
		}
	}
	return jobs
}
