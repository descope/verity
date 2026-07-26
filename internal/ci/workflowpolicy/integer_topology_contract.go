package workflowpolicy

import (
	"slices"
	"strings"
)

const integerBuildVerityReference = protectedBuildVerityWorkflowReference

type integerDirectWrapperSpec struct {
	name              string
	implementationJob string
	implementation    string
	push              bool
	schedule          bool
}

var integerDirectWrapperSpecs = []integerDirectWrapperSpec{
	{
		name:              "integer-orchestrator.yaml",
		implementationJob: "orchestrate",
		implementation:    integerOrchestratorReference,
		push:              true,
		schedule:          true,
	},
	{
		name:              "integer-build-image.yaml",
		implementationJob: "build",
		implementation:    integerImageWorkflowReference,
	},
}

var integerStrictEntryJobs = map[string]string{
	"integer-orchestrator-reusable.yaml": "plan",
	"integer-build-shard.yaml":           "build",
	"integer-build-image-reusable.yaml":  "melange-prep",
}

var integerCoordinateInputs = []string{
	"source_sha",
	"verity_artifact_name",
	"verity_artifact_digest",
	"verity_build_key",
}

func validateIntegerStructuralTopology(workflows []workflowFile) []Violation {
	violations := make([]Violation, 0, len(integerDirectWrapperSpecs)+len(integerStrictEntryJobs)+1)
	for _, spec := range integerDirectWrapperSpecs {
		violations = append(violations, validateIntegerDirectWrapper(workflows, spec)...)
	}
	for name, entryJob := range integerStrictEntryJobs {
		violations = append(violations, validateIntegerStrictWorkflow(workflows, name, entryJob)...)
	}
	violations = append(violations, validateIntegerMatrixTopology(workflows)...)
	return violations
}

func validateIntegerDirectWrapper(workflows []workflowFile, spec integerDirectWrapperSpec) []Violation {
	file, exists := findWorkflow(workflows, spec.name)
	if !exists {
		return []Violation{integerViolation(spec.name, "", "required direct wrapper is missing")}
	}
	violations := validateIntegerDirectWrapperTriggers(&file, spec)
	return append(violations, validateIntegerDirectWrapperJobs(&file, spec)...)
}

func validateIntegerDirectWrapperTriggers(file *workflowFile, spec integerDirectWrapperSpec) []Violation {
	var violations []Violation
	triggers := file.Workflow.On
	if triggers.WorkflowCall || !triggers.WorkflowDispatch || triggers.Push.Present != spec.push || triggers.Schedule != spec.schedule {
		violations = append(violations, integerViolation(spec.name, "", "public Integer wrapper must expose only its direct triggers"))
	}
	return violations
}

func validateIntegerDirectWrapperJobs(file *workflowFile, spec integerDirectWrapperSpec) []Violation {
	violations := make([]Violation, 0, 3)
	if len(file.Workflow.Jobs) != 2 {
		violations = append(violations, integerViolation(spec.name, "", "public Integer wrapper must contain exactly one builder and one implementation call"))
	}
	builder, builderExists := file.Workflow.Jobs["build-verity"]
	if !builderExists || builder.Uses != integerBuildVerityReference || !hasSuccessSemantics(builder.If) ||
		normalizeExpression(builder.With["source_sha"]) != "${{github.sha}}" || len(builder.With) != 1 {
		violations = append(violations, integerViolation(spec.name, "build-verity", "direct wrapper must build protected Verity exactly once"))
	}
	implementation, implementationExists := file.Workflow.Jobs[spec.implementationJob]
	expectedCoordinates := map[string]string{
		"source_sha":             "${{needs.build-verity.outputs.source-sha}}",
		"verity_artifact_name":   "${{needs.build-verity.outputs.artifact-name}}",
		"verity_artifact_digest": "${{needs.build-verity.outputs.artifact-digest}}",
		"verity_build_key":       "${{needs.build-verity.outputs.build-key}}",
	}
	if !implementationExists || implementation.Uses != spec.implementation ||
		!slices.Equal([]string(implementation.Needs), []string{"build-verity"}) ||
		!integerWithMatches(implementation.With, expectedCoordinates) {
		violations = append(violations, integerViolation(spec.name, spec.implementationJob, "direct wrapper must pass the one builder's complete coordinates to the strict implementation"))
	}
	return violations
}

func validateIntegerStrictWorkflow(workflows []workflowFile, name, entryJob string) []Violation {
	file, exists := findWorkflow(workflows, name)
	if !exists {
		return nil
	}
	var violations []Violation
	for jobName := range file.Workflow.Jobs {
		job := file.Workflow.Jobs[jobName]
		if job.Uses == integerBuildVerityReference {
			violations = append(violations, integerViolation(name, jobName, "strict reusable Integer workflow must never self-build Verity"))
		}
		if !integerSetupPrecedesUse(job.Steps) {
			violations = append(violations, integerViolation(name, jobName, "every Verity invocation must follow setup-verity in the same job"))
		}
	}
	validator, validatorExists := file.Workflow.Jobs["validate-coordinates"]
	if !validatorExists || strings.TrimSpace(validator.If) != "" || !integerValidatorRejectsIncompleteCoordinates(validator.Steps) {
		violations = append(violations, integerViolation(name, "validate-coordinates", "strict reusable workflow must reject every incomplete Verity coordinate set"))
	}
	entry, entryExists := file.Workflow.Jobs[entryJob]
	if !entryExists || !slices.Contains([]string(entry.Needs), "validate-coordinates") {
		violations = append(violations, integerViolation(name, entryJob, "strict implementation work must wait for coordinate validation"))
	}
	return violations
}

func integerValidatorRejectsIncompleteCoordinates(steps []workflowStep) bool {
	for stepIndex := range steps {
		step := &steps[stepIndex]
		if step.Name != "Reject incomplete Verity coordinates" || strings.TrimSpace(step.Run) != "exit 1" {
			continue
		}
		clauses := strings.Split(strings.Join(strings.Fields(step.If), ""), "||")
		if len(clauses) != len(integerCoordinateInputs) {
			return false
		}
		for _, name := range integerCoordinateInputs {
			if !slices.Contains(clauses, "inputs."+name+"==''") {
				return false
			}
		}
		return true
	}
	return false
}

func integerSetupPrecedesUse(steps []workflowStep) bool {
	setup := false
	for stepIndex := range steps {
		step := &steps[stepIndex]
		if step.Uses == "./.github/actions/setup-verity" {
			setup = true
		}
		if strings.Contains(step.Run, "./verity") && !setup {
			return false
		}
	}
	return true
}

func validateIntegerMatrixTopology(workflows []workflowFile) []Violation {
	checks := []struct {
		workflow string
		job      string
		needs    string
	}{
		{workflow: "integer-orchestrator-reusable.yaml", job: "build-shards", needs: "plan"},
		{workflow: "integer-build-shard.yaml", job: "build", needs: "validate-coordinates"},
		{workflow: "integer-build-image-reusable.yaml", job: "melange-build", needs: "melange-prep"},
	}
	var violations []Violation
	for _, check := range checks {
		file, exists := findWorkflow(workflows, check.workflow)
		job, jobExists := file.Workflow.Jobs[check.job]
		if !exists || !jobExists || job.Strategy.Matrix.Kind == 0 || !slices.Contains([]string(job.Needs), check.needs) {
			violations = append(violations, integerViolation(check.workflow, check.job, "matrix fan-out must follow validated planning and consume prebuilt Verity"))
		}
	}
	return violations
}
