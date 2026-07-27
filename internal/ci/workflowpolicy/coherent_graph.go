package workflowpolicy

import (
	"fmt"
)

type integrationCallContract struct {
	workflowName string
	jobName      string
	chartName    string
	sourceName   string
	job          *workflowJob
}

func validateCoherentGraph(workflows []workflowFile) []Violation {
	orchestrator, exists := findWorkflow(workflows, copaOrchestratorWorkflow)
	if !exists {
		return nil
	}

	patchName, patch, patchFound := findReusableJob(&orchestrator, copaPatchReference)
	chartName, chart, chartFound := findReusableJob(&orchestrator, chartProducerReference)
	integrationName, integration, integrationFound := findReusableJob(&orchestrator, chartIntegrationReference)
	privilegedName, privileged, privilegedFound := findReusableJob(&orchestrator, privilegedChartReference)

	var violations []Violation
	if !patchFound || !chartFound || !integrationFound || !privilegedFound {
		return []Violation{coherentViolation(orchestrator.Name, "", "orchestrator must call patch, chart, and both chart integration reusable producers")}
	}

	sourceJob, reason := exactNeedsProducer(&patch, []string{"source_sha", "run_id", "run_attempt", "publication_id", "batch_id"})
	if reason != "" || !containsString(patch.Needs, sourceJob) {
		violations = append(violations, coherentViolation(orchestrator.Name, patchName, "patch producer must consume exact identity from its explicit needs producer"))
	}
	chartSource, reason := exactNeedsProducer(&chart, coherentProducerOutputs)
	if reason != "" || chartSource != sourceJob || !containsString(chart.Needs, sourceJob) || !containsString(chart.Needs, patchName) {
		violations = append(violations, coherentViolation(orchestrator.Name, chartName, "chart producer must need the successful patch graph and exact COPA artifact identity"))
	}
	if !gatesEquivalent(chart.If, expectedChartGate(sourceJob, patchName)) {
		violations = append(violations, coherentViolation(orchestrator.Name, chartName, "chart producer must not run after failed or cancelled COPA producers"))
	}

	violations = append(violations, validateIntegrationCall(integrationCallContract{
		workflowName: orchestrator.Name, jobName: integrationName, job: &integration,
		chartName: chartName, sourceName: sourceJob,
	})...)
	violations = append(violations, validateIntegrationCall(integrationCallContract{
		workflowName: orchestrator.Name, jobName: privilegedName, job: &privileged,
		chartName: chartName, sourceName: sourceJob,
	})...)
	return violations
}

func findReusableJob(file *workflowFile, reference string) (string, workflowJob, bool) {
	for _, jobName := range sortedJobNames(file.Workflow.Jobs) {
		job := file.Workflow.Jobs[jobName]
		if job.Uses == reference {
			return jobName, job, true
		}
	}
	return "", workflowJob{}, false
}

func exactNeedsProducer(job *workflowJob, fields []string) (producerName, reason string) {
	for _, field := range fields {
		name, exact := exactOutputReference(job.With[field], "needs", field)
		if !exact {
			return "", field + " is not an exact needs output"
		}
		if producerName == "" {
			producerName = name
		} else if producerName != name {
			return "", "identity fields come from different producers"
		}
	}
	return producerName, ""
}

func validateIntegrationCall(contract integrationCallContract) []Violation {
	producerName, reason := exactNeedsProducer(contract.job, coherentProducerOutputs)
	if reason != "" || producerName != contract.chartName ||
		!containsString(contract.job.Needs, contract.chartName) ||
		!containsString(contract.job.Needs, contract.sourceName) {
		return []Violation{coherentViolation(contract.workflowName, contract.jobName, "chart integration must consume exact chart outputs through explicit needs")}
	}

	if !gatesEquivalent(contract.job.If, expectedIntegrationCallGate(contract.chartName, contract.sourceName)) {
		return []Violation{coherentViolation(contract.workflowName, contract.jobName, "chart integration gate must require successful same-source producer identity")}
	}
	return nil
}

func expectedChartGate(sourceName, patchName string) string {
	return fmt.Sprintf(
		"needs.%[1]s.result == 'success' && "+
			"(needs.%[2]s.result == 'success' || needs.%[2]s.result == 'skipped') && "+
			"needs.%[1]s.outputs.source_sha == github.sha && "+
			"needs.%[1]s.outputs.run_id == github.run_id && "+
			"needs.%[1]s.outputs.run_attempt == github.run_attempt && "+
			"needs.%[1]s.outputs.publication_id != '' && "+
			"needs.%[1]s.outputs.batch_id != '' && "+
			"needs.%[1]s.outputs.artifact_name != '' && "+
			"needs.%[1]s.outputs.artifact_digest != ''",
		sourceName,
		patchName,
	)
}

func expectedIntegrationCallGate(chartName, sourceName string) string {
	return fmt.Sprintf(
		"needs.%[1]s.result == 'success' && "+
			"needs.%[1]s.outputs.source_sha == needs.%[2]s.outputs.source_sha && "+
			"needs.%[1]s.outputs.run_id == needs.%[2]s.outputs.run_id && "+
			"needs.%[1]s.outputs.run_attempt == needs.%[2]s.outputs.run_attempt && "+
			"needs.%[1]s.outputs.publication_id == needs.%[2]s.outputs.publication_id && "+
			"needs.%[1]s.outputs.batch_id == needs.%[2]s.outputs.batch_id && "+
			"needs.%[1]s.outputs.artifact_name != '' && "+
			"needs.%[1]s.outputs.artifact_digest != ''",
		chartName,
		sourceName,
	)
}
