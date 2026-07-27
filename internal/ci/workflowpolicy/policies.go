package workflowpolicy

import "sort"

func evaluatePolicies(directory string, workflows []workflowFile) []Violation {
	violations := make([]Violation, 0, 32)
	violations = append(violations, validatePagesOwnership(workflows)...)
	violations = append(violations, validatePermissions(workflows)...)
	violations = append(violations, validateReusableJobSecrets(workflows)...)
	violations = append(violations, validatePinnedReferences(workflows)...)
	violations = append(violations, validateProtectedDispatchRefs(workflows)...)
	violations = append(violations, validateAPKSigningBoundary(workflows)...)
	violations = append(violations, validateSignerProvenance(workflows)...)
	violations = append(violations, validateMelangeArtifactPaths(workflows)...)
	violations = append(violations, validateIntegerContract(workflows)...)
	violations = append(violations, validateCoherentProducerChain(workflows)...)
	violations = append(violations, validateGoOwnedLogic(workflows)...)
	violations = append(violations, validateZeroCVEOrdering(workflows)...)
	violations = append(violations, validateLocalVerityBuildPolicy(directory, workflows)...)
	violations = append(violations, validateBuildOnceDirectory(directory, workflows)...)
	violations = append(violations, validatePublicationRollout(directory, workflows)...)
	return violations
}

func findWorkflow(workflows []workflowFile, name string) (workflowFile, bool) {
	for index := range workflows {
		file := &workflows[index]
		if file.Name == name {
			return *file, true
		}
	}
	return workflowFile{}, false
}

func sortedJobNames(jobs map[string]workflowJob) []string {
	names := make([]string, 0, len(jobs))
	for name := range jobs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func actionName(reference string) string {
	for index := len(reference) - 1; index >= 0; index-- {
		if reference[index] == '@' {
			return reference[:index]
		}
	}
	return reference
}
