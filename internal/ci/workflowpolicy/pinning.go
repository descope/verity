package workflowpolicy

import (
	"fmt"
	"path"
	"regexp"
	"strings"
)

var commitReferencePattern = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)

func validatePinnedReferences(workflows []workflowFile) []Violation {
	var violations []Violation
	for index := range workflows {
		file := &workflows[index]
		for _, jobName := range sortedJobNames(file.Workflow.Jobs) {
			job := file.Workflow.Jobs[jobName]
			violations = append(violations, validateActionReference(file.Name, jobName, job.Uses)...)
			violations = append(violations, validateImageReference(file.Name, jobName, job.Container.Image)...)
			for _, service := range job.Services {
				violations = append(violations, validateImageReference(file.Name, jobName, service.Image)...)
			}
			for stepIndex := range job.Steps {
				step := &job.Steps[stepIndex]
				violations = append(violations, validateActionReference(file.Name, jobName, step.Uses)...)
				for _, image := range inlineContainerImages(step.Run) {
					violations = append(violations, validateImageReference(file.Name, jobName, image)...)
				}
			}
		}
	}
	return violations
}

func validateActionReference(workflowName, jobName, reference string) []Violation {
	if reference == "" {
		return nil
	}
	if reference != strings.TrimSpace(reference) {
		return nonCanonicalActionViolation(workflowName, jobName, reference)
	}
	if strings.HasPrefix(reference, "./") {
		if canonicalLocalReference(reference) {
			return nil
		}
		return nonCanonicalActionViolation(workflowName, jobName, reference)
	}
	if image, ok := strings.CutPrefix(reference, "docker://"); ok {
		return validateImageReference(workflowName, jobName, image)
	}
	separator := strings.LastIndex(reference, "@")
	if separator > 0 && canonicalExternalAction(reference[:separator]) &&
		commitReferencePattern.MatchString(reference[separator+1:]) {
		return nil
	}
	if separator > 0 && !canonicalExternalAction(reference[:separator]) {
		return nonCanonicalActionViolation(workflowName, jobName, reference)
	}
	return []Violation{{
		Rule: RulePinnedReference, Workflow: workflowName, Job: jobName,
		Detail: fmt.Sprintf("external action %q must use a full commit SHA", reference),
	}}
}

func canonicalLocalReference(reference string) bool {
	if reference != strings.ToLower(reference) || strings.Contains(reference, "\\") || strings.Contains(reference, "@") {
		return false
	}
	target := strings.TrimPrefix(reference, "./")
	return target != "" && target != "." && !strings.HasPrefix(target, "../") && path.Clean(target) == target
}

func canonicalExternalAction(target string) bool {
	if target != strings.ToLower(target) || strings.Contains(target, "\\") || path.Clean(target) != target {
		return false
	}
	segments := strings.Split(target, "/")
	return len(segments) >= 2 && segments[0] != "" && segments[1] != ""
}

func nonCanonicalActionViolation(workflowName, jobName, reference string) []Violation {
	return []Violation{{
		Rule: RulePinnedReference, Workflow: workflowName, Job: jobName,
		Detail: fmt.Sprintf("action reference %q must use canonical lowercase path syntax", reference),
	}}
}

func validateImageReference(workflowName, jobName, image string) []Violation {
	if image == "" || imagePinnedByDigest(image) {
		return nil
	}
	return []Violation{{
		Rule: RulePinnedReference, Workflow: workflowName, Job: jobName,
		Detail: fmt.Sprintf("container image %q must use a sha256 digest", image),
	}}
}

func imagePinnedByDigest(image string) bool {
	separator := strings.LastIndex(image, "@sha256:")
	if separator < 1 || len(image[separator+len("@sha256:"):]) != 64 {
		return false
	}
	for _, character := range image[separator+len("@sha256:"):] {
		if !strings.ContainsRune("0123456789abcdefABCDEF", character) {
			return false
		}
	}
	return true
}
