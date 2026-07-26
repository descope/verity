package workflowpolicy

import (
	"fmt"
	"strings"
)

const (
	protectedBuildVerityWorkflowName      = "build-verity-protected.yaml"
	protectedBuildVerityWorkflowReference = "./.github/workflows/build-verity-protected.yaml"
	protectedBuildGate                    = "${{github.repository=='verity-org/verity'&&github.ref_protected==true&&inputs.source_sha==github.sha}}"
)

func validateProtectedBuildWorkflow(name string, data []byte) ([]Violation, error) {
	parsed, err := decodeWorkflow(data)
	if err != nil {
		return nil, fmt.Errorf("decode protected build workflow %q: %w", name, err)
	}
	file := workflowFile{Name: name, Workflow: parsed}
	violations := validateProtectedBuildSurface(&file, data)
	violations = append(violations, validateProtectedBuildJobs(&file)...)
	violations = append(violations, validatePinnedReferences([]workflowFile{file})...)
	if strings.Contains(string(data), "${{ secrets.") {
		violations = append(violations, buildOnceViolation(name, "", "protected build workflow must not accept secrets"))
	}
	return violations, nil
}

func validateProtectedBuildSurface(file *workflowFile, data []byte) []Violation {
	var violations []Violation
	if file.Name != protectedBuildVerityWorkflowName {
		violations = append(violations, buildOnceViolation(file.Name, "", "protected producer must use its canonical workflow name"))
	}
	if !file.Workflow.Permissions.declared || file.Workflow.Permissions.all != "" || len(file.Workflow.Permissions.scopes) != 0 {
		violations = append(violations, buildOnceViolation(file.Name, "", "protected workflow defaults must deny all permissions"))
	}
	inputs, triggerCount, err := decodeBuildOnceInputs(data)
	if err != nil || triggerCount != 1 || !exactBuildOnceInputs(inputs) {
		violations = append(violations, buildOnceViolation(file.Name, "", "protected workflow_call must accept only required source_sha"))
	}
	if len(file.Workflow.On.WorkflowSecrets) != 0 {
		violations = append(violations, buildOnceViolation(file.Name, "", "protected build workflow must not accept secrets"))
	}
	if !exactBuildOnceOutputs(file.Workflow.On.WorkflowOutputs) {
		violations = append(violations, buildOnceViolation(file.Name, "", "protected outputs must proxy the exact build coordinates"))
	}
	return violations
}

func validateProtectedBuildJobs(file *workflowFile) []Violation {
	var violations []Violation
	if len(file.Workflow.Jobs) != 2 {
		violations = append(violations, buildOnceViolation(file.Name, "", "protected workflow must contain only build and attest jobs"))
	}
	build, buildExists := file.Workflow.Jobs["build"]
	if !buildExists || build.Uses != buildVerityWorkflowReference || normalizeExpression(build.If) != protectedBuildGate ||
		!buildOnceReadPermissions(build.Permissions) || normalizeExpression(build.With["source_sha"]) != "${{inputs.source_sha}}" {
		violations = append(violations, buildOnceViolation(file.Name, "build", "protected build must call the unprivileged producer for the exact protected SHA"))
	}
	attest, attestExists := file.Workflow.Jobs["attest"]
	if !attestExists {
		violations = append(violations, buildOnceViolation(file.Name, "attest", "protected attestation job is required"))
	} else {
		violations = append(violations, validateProtectedAttestJob(file.Name, &attest)...)
	}
	return violations
}

func validateProtectedAttestJob(name string, job *workflowJob) []Violation {
	var violations []Violation
	if len(job.Needs) != 1 || job.Needs[0] != "build" {
		violations = append(violations, buildOnceViolation(name, "attest", "attestation must depend only on the protected build"))
	}
	expected := map[permissionScope]permissionLevel{
		actionsScope: permissionRead, contentsScope: permissionRead,
		idTokenScope: permissionWrite, attestationsScope: permissionWrite,
	}
	if !buildOnceExactPermissions(job.Permissions, expected) {
		violations = append(violations, buildOnceViolation(name, "attest", "attestation writes must exist only on the protected attestation job"))
	}
	if !exactProtectedAttestationSteps(job.Steps) {
		violations = append(violations, buildOnceViolation(name, "attest", "attestation must consume and attest the exact protected binary"))
	}
	return violations
}

func exactProtectedAttestationSteps(steps []workflowStep) bool {
	setupFound := false
	attestFound := false
	for index := range steps {
		step := &steps[index]
		switch actionName(step.Uses) {
		case "./.github/actions/setup-verity":
			setupFound = normalizeExpression(step.With["artifact-name"]) == "${{needs.build.outputs.artifact-name}}" &&
				normalizeExpression(step.With["artifact-digest"]) == "${{needs.build.outputs.artifact-digest}}" &&
				normalizeExpression(step.With["source-sha"]) == "${{needs.build.outputs.source-sha}}" &&
				normalizeExpression(step.With["build-key"]) == "${{needs.build.outputs.build-key}}" &&
				step.With["verify-attestation"] == "false"
		case "actions/attest-build-provenance":
			attestFound = step.With["subject-path"] == "verity"
		}
	}
	return setupFound && attestFound
}
