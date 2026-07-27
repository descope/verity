package workflowpolicy

const (
	integerInputSurfaceDetail  = "workflow_call inputs must contain the exact producer contract"
	integerOutputSurfaceDetail = "workflow_call outputs must contain only exact artifact names and digests"
	workflowInputStringType    = "string"
	workflowInputBooleanType   = "boolean"
)

func validateIntegerSurface(file *workflowFile, surface integerSurface) []Violation {
	violations := make([]Violation, 0, 4)
	if !file.Workflow.On.WorkflowCall || file.Workflow.On.Push.Present ||
		file.Workflow.On.Schedule || file.Workflow.On.WorkflowDispatch {
		violations = append(violations, integerViolation(file.Name, "", "workflow_call contract is required"))
	}
	if file.Workflow.On.PullRequest || file.Workflow.On.PullRequestTarget ||
		file.Workflow.Permissions.level(contentsScope) != permissionRead {
		violations = append(violations, integerViolation(
			file.Name,
			"",
			"reusable Integer defaults must remain pull-request read-only",
		))
	}
	violations = append(violations, validateIntegerRequiredInputs(file, surface)...)
	violations = append(violations, validateIntegerOptionalInputs(file, surface)...)
	return append(violations, validateIntegerOutputs(file, surface)...)
}

func validateIntegerRequiredInputs(file *workflowFile, surface integerSurface) []Violation {
	if len(file.Workflow.On.WorkflowInputs) != len(surface.inputs)+len(surface.optionalInputs) {
		return []Violation{integerViolation(file.Name, "", integerInputSurfaceDetail)}
	}
	for name, inputType := range surface.inputs {
		input, exists := file.Workflow.On.WorkflowInputs[name]
		if !exists || !input.Required || input.Type != inputType {
			return []Violation{integerViolation(file.Name, "", integerInputSurfaceDetail)}
		}
	}
	return nil
}

func validateIntegerOptionalInputs(file *workflowFile, surface integerSurface) []Violation {
	for name, inputType := range surface.optionalInputs {
		input, exists := file.Workflow.On.WorkflowInputs[name]
		if !exists || input.Required || input.Type != inputType {
			return []Violation{integerViolation(file.Name, "", integerInputSurfaceDetail)}
		}
	}
	return nil
}

func validateIntegerOutputs(file *workflowFile, surface integerSurface) []Violation {
	if len(file.Workflow.On.WorkflowOutputs) != len(surface.outputs) {
		return []Violation{integerViolation(file.Name, "", integerOutputSurfaceDetail)}
	}
	for _, outputName := range surface.outputs {
		output, exists := file.Workflow.On.WorkflowOutputs[outputName]
		job, exact := exactOutputReference(output.Value, "jobs", outputName)
		if !exists || !exact || job != surface.outputJob {
			return []Violation{integerViolation(file.Name, "", integerOutputSurfaceDetail)}
		}
	}
	return nil
}
