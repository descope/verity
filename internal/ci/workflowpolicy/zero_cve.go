package workflowpolicy

import "strings"

const allTrivySeverities = "UNKNOWN,LOW,MEDIUM,HIGH,CRITICAL"

type zeroCVEPublication struct {
	workflowName string
	jobName      string
	ordered      string
	position     int
	publishStep  workflowStep
	publishRun   string
}

func validateZeroCVEOrdering(workflows []workflowFile) []Violation {
	file, ok := findWorkflow(workflows, "integer-build-image-reusable.yaml")
	if !ok {
		return []Violation{{
			Rule: RuleZeroCVEOrdering, Workflow: "integer-build-image-reusable.yaml",
			Detail: "required workflow is missing",
		}}
	}

	var violations []Violation
	foundPublication := false
	for _, jobName := range sortedJobNames(file.Workflow.Jobs) {
		job := file.Workflow.Jobs[jobName]
		jobViolations, found := validateZeroCVEJob(file.Name, jobName, &job)
		violations = append(violations, jobViolations...)
		foundPublication = foundPublication || found
	}
	if !foundPublication {
		violations = append(violations, Violation{
			Rule: RuleZeroCVEOrdering, Workflow: file.Name,
			Detail: "Go-owned staged Trivy publication is missing",
		})
	}
	return violations
}

func validateZeroCVEJob(workflowName, jobName string, job *workflowJob) ([]Violation, bool) {
	ordered := orderedStepText(job.Steps)
	position := strings.Index(ordered, "./verity ci integer-image publish")
	if position < 0 {
		return nil, false
	}
	publishStep, publishRun := findStepContaining(job.Steps, "./verity ci integer-image publish")
	publication := zeroCVEPublication{
		workflowName: workflowName,
		jobName:      jobName,
		ordered:      ordered,
		position:     position,
		publishStep:  publishStep,
		publishRun:   publishRun,
	}
	return validateZeroCVEPublication(&publication), true
}

func validateZeroCVEPublication(publication *zeroCVEPublication) []Violation {
	gate := strings.Index(publication.ordered, "--fail-on-severity "+allTrivySeverities)
	if gate < 0 || gate > publication.position {
		return zeroCVEViolation(publication, "the all-severity local Trivy gate must run before Go-owned publication")
	}
	violations := make([]Violation, 0, 4)
	violations = append(violations, validateZeroCVEProvenance(publication)...)
	violations = append(violations, validateZeroCVEContinueOnError(publication)...)
	violations = append(violations, validateZeroCVEExternalPublication(publication)...)
	violations = append(violations, validateZeroCVEArtifactOrdering(publication)...)
	return violations
}

func validateZeroCVEProvenance(publication *zeroCVEPublication) []Violation {
	for _, flag := range []string{"--source-sha", "--run-id", "--run-attempt", "--config", "--tags", "--github-output"} {
		if strings.Contains(publication.publishRun, flag) {
			continue
		}
		return zeroCVEViolation(publication, "Go-owned publication must receive exact provenance and output flags")
	}
	return nil
}

func validateZeroCVEContinueOnError(publication *zeroCVEPublication) []Violation {
	if publication.publishStep.ContinueOnError.set && !strings.EqualFold(publication.publishStep.ContinueOnError.value, "false") ||
		strings.Contains(publication.publishRun, "|| true") {
		return zeroCVEViolation(publication, "Go-owned staged Trivy publication must not continue on error")
	}
	return nil
}

func validateZeroCVEExternalPublication(publication *zeroCVEPublication) []Violation {
	if !strings.Contains(publication.ordered, "apko publish") &&
		!strings.Contains(publication.ordered, "crane copy") &&
		!strings.Contains(publication.ordered, "trivy image") {
		return nil
	}
	return zeroCVEViolation(publication, "staged scan and final promotion must remain inside the typed Go publisher")
}

func validateZeroCVEArtifactOrdering(publication *zeroCVEPublication) []Violation {
	for _, marker := range []string{"actions/attest-build-provenance@", "integer-component-${{"} {
		position := strings.Index(publication.ordered, marker)
		if position >= 0 && position < publication.position {
			return zeroCVEViolation(publication, "APK attestation and upload must remain after the zero-CVE gate")
		}
	}
	return nil
}

func zeroCVEViolation(publication *zeroCVEPublication, detail string) []Violation {
	return []Violation{{
		Rule: RuleZeroCVEOrdering, Workflow: publication.workflowName, Job: publication.jobName,
		Detail: detail,
	}}
}

func orderedStepText(steps []workflowStep) string {
	var builder strings.Builder
	for stepIndex := range steps {
		step := &steps[stepIndex]
		builder.WriteString(step.Uses)
		builder.WriteByte('\n')
		builder.WriteString(step.Run)
		builder.WriteByte('\n')
		builder.WriteString(step.With["name"])
		builder.WriteByte('\n')
	}
	return builder.String()
}

func findStepContaining(steps []workflowStep, marker string) (matched workflowStep, run string) {
	for stepIndex := range steps {
		step := &steps[stepIndex]
		if strings.Contains(step.Run, marker) {
			return *step, step.Run
		}
	}
	return workflowStep{}, ""
}
