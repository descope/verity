package workflowpolicy

const buildVerityWorkflowReference = "./.github/workflows/build-verity.yaml"

var setupVerityIdentityOutputs = map[string]string{
	"artifact-name":   "artifact-name",
	"artifact-digest": "artifact-digest",
	"source-sha":      "source-sha",
	"build-key":       "build-key",
}

func (activation signerActivation) exactReadOnlySameRunActivation(stepIndex int) bool {
	if !explicitlyReadOnly(activation.file.Workflow.Permissions, activation.job.Permissions) ||
		jobUsesSecretContext(activation.file.Workflow.Env, activation.job) ||
		isSigningJob(activation.file.Name, activation.jobName, activation.job) {
		return false
	}

	step := &activation.job.Steps[stepIndex]
	producerName := ""
	for inputName, outputName := range setupVerityIdentityOutputs {
		producer, exact := exactOutputReference(step.With[inputName], "needs", outputName)
		if !exact || producerName != "" && producerName != producer {
			return false
		}
		producerName = producer
	}
	producer, exists := activation.file.Workflow.Jobs[producerName]
	return exists && containsString(activation.job.Needs, producerName) &&
		producer.Uses == buildVerityWorkflowReference &&
		explicitlyReadOnly(activation.file.Workflow.Permissions, producer.Permissions) &&
		exactGitHubSHAExpression(producer.With["source_sha"])
}
