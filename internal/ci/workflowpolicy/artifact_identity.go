package workflowpolicy

import (
	"regexp"
)

var latestApprovedDownload = regexp.MustCompile(`(?i)apk-repository\s+download-approved\s+["']?latest(?:["']|\s|$)`)

func validateCrossRunDownloads(workflows []workflowFile) []Violation {
	var violations []Violation
	for index := range workflows {
		file := &workflows[index]
		if coherentArtifactConsumer(file.Name) {
			continue
		}
		for _, jobName := range sortedJobNames(file.Workflow.Jobs) {
			job := file.Workflow.Jobs[jobName]
			for stepIndex := range job.Steps {
				step := &job.Steps[stepIndex]
				if latestApprovedDownload.MatchString(step.Run) {
					violations = append(violations, Violation{
						Rule: RuleProducerIdentity, Workflow: file.Name, Job: jobName,
						Detail: "approved APK downloads must identify an exact Integer run and attempt",
					})
				}
				if actionName(step.Uses) != "actions/download-artifact" || !isCrossRunDownload(step) {
					continue
				}
				context := downloadProducerContext{
					workflows: workflows,
					file:      file,
					consumer:  &job,
					step:      step,
					stepIndex: stepIndex,
				}
				if reason := validateDownloadProducer(context); reason != "" {
					violations = append(violations, Violation{
						Rule: RuleProducerIdentity, Workflow: file.Name, Job: jobName,
						Detail: reason,
					})
				}
			}
		}
	}
	return violations
}

func isCrossRunDownload(step *workflowStep) bool {
	return step.With["github-token"] != "" || step.With["repository"] != "" || step.With["run-id"] != ""
}
