package workflowpolicy

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	buildVerityWorkflowName = "build-verity.yaml"
	buildVerityCommand      = "go run ./.github/actions/setup-verity/cmd/setup-verity build"
	protectedAttestationIf  = "${{inputs.protected_attestation&&github.repository=='verity-org/verity'&&github.ref_protected}}"
)

type rawWorkflowCallInput struct {
	Required bool   `yaml:"required"`
	Type     string `yaml:"type"`
	Default  *bool  `yaml:"default"`
}

func validateBuildOnceWorkflow(name string, data []byte) ([]Violation, error) {
	parsed, err := decodeWorkflow(data)
	if err != nil {
		return nil, fmt.Errorf("decode build-once workflow %q: %w", name, err)
	}
	file := workflowFile{Name: name, Workflow: parsed}
	violations := make([]Violation, 0, 16)
	violations = append(violations, validateBuildOnceSurface(&file, data)...)
	violations = append(violations, validateBuildOnceJobs(&file)...)
	violations = append(violations, validatePinnedReferences([]workflowFile{file})...)
	if strings.Contains(string(data), "${{ secrets.") {
		violations = append(violations, buildOnceViolation(name, "", "secret exposure is forbidden in the build-once workflow"))
	}
	if strings.Contains(string(data), "actions/cache/") {
		violations = append(violations, buildOnceViolation(name, "", "no executable cache may be trusted"))
	}
	return violations, nil
}

func validateBuildOnceSurface(file *workflowFile, data []byte) []Violation {
	var violations []Violation
	if file.Name != buildVerityWorkflowName {
		violations = append(violations, buildOnceViolation(file.Name, "", "reusable producer must be named build-verity.yaml"))
	}
	if !buildOnceReadPermissions(file.Workflow.Permissions) {
		violations = append(violations, buildOnceViolation(file.Name, "", "workflow defaults must grant only contents: read"))
	}

	inputs, triggerCount, err := decodeBuildOnceInputs(data)
	if err != nil {
		violations = append(violations, buildOnceViolation(file.Name, "", "workflow_call inputs must be canonical"))
	} else if triggerCount != 1 || !exactBuildOnceInputs(inputs) {
		violations = append(violations, buildOnceViolation(file.Name, "", "workflow_call inputs must be required source_sha and optional protected_attestation"))
	}
	if input := inputs["protected_attestation"]; input.Default == nil || *input.Default {
		violations = append(violations, buildOnceViolation(file.Name, "", "protected attestation default must remain false"))
	}
	if len(file.Workflow.On.WorkflowSecrets) != 0 {
		violations = append(violations, buildOnceViolation(file.Name, "", "build-once workflow must not accept secrets"))
	}
	if !exactBuildOnceOutputs(file.Workflow.On.WorkflowOutputs) {
		violations = append(violations, buildOnceViolation(file.Name, "", "workflow_call outputs must bind artifact name, digest, source SHA, and build key"))
	}
	return violations
}

func decodeBuildOnceInputs(data []byte) (inputs map[string]rawWorkflowCallInput, triggerCount int, err error) {
	var raw struct {
		On map[string]yaml.Node `yaml:"on"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, 0, fmt.Errorf("decode workflow triggers: %w", err)
	}
	call, exists := raw.On["workflow_call"]
	if !exists {
		return nil, len(raw.On), nil
	}
	var contract struct {
		Inputs map[string]rawWorkflowCallInput `yaml:"inputs"`
	}
	if err := call.Decode(&contract); err != nil {
		return nil, len(raw.On), fmt.Errorf("decode workflow_call inputs: %w", err)
	}
	return contract.Inputs, len(raw.On), nil
}

func exactBuildOnceInputs(inputs map[string]rawWorkflowCallInput) bool {
	if len(inputs) != 2 {
		return false
	}
	source, sourceExists := inputs["source_sha"]
	protected, protectedExists := inputs["protected_attestation"]
	return sourceExists && source.Required && source.Type == "string" && source.Default == nil &&
		protectedExists && !protected.Required && protected.Type == "boolean"
}

func exactBuildOnceOutputs(outputs map[string]workflowCallOutput) bool {
	if len(outputs) != 4 {
		return false
	}
	return normalizeExpression(outputs["artifact-name"].Value) == "${{jobs.build.outputs.artifact-name}}" &&
		normalizeExpression(outputs["artifact-digest"].Value) == "${{jobs.build.outputs.artifact-digest}}" &&
		normalizeExpression(outputs["source-sha"].Value) == "${{jobs.build.outputs.source-sha}}" &&
		normalizeExpression(outputs["build-key"].Value) == "${{jobs.build.outputs.build-key}}"
}

func validateBuildOnceJobs(file *workflowFile) []Violation {
	var violations []Violation
	if len(file.Workflow.Jobs) != 2 {
		violations = append(violations, buildOnceViolation(file.Name, "", "build-once workflow must contain only build and attest jobs"))
	}
	build, buildExists := file.Workflow.Jobs["build"]
	attest, attestExists := file.Workflow.Jobs["attest"]
	if !buildExists || !buildOnceReadPermissions(build.Permissions) {
		violations = append(violations, buildOnceViolation(file.Name, "build", "build job must remain contents-read only"))
	} else {
		violations = append(violations, validateBuildJob(file.Name, &build)...)
	}
	if !attestExists {
		violations = append(violations, buildOnceViolation(file.Name, "attest", "protected attestation job is required"))
	} else {
		violations = append(violations, validateAttestJob(file.Name, &attest)...)
	}
	return violations
}

func validateBuildJob(name string, job *workflowJob) []Violation {
	var violations []Violation
	if len(job.Outputs) != 4 ||
		normalizeExpression(job.Outputs["artifact-name"]) != "${{steps.build.outputs.artifact-name}}" ||
		normalizeExpression(job.Outputs["artifact-digest"]) != "${{steps.upload.outputs.artifact-digest}}" ||
		normalizeExpression(job.Outputs["source-sha"]) != "${{steps.build.outputs.source-sha}}" ||
		normalizeExpression(job.Outputs["build-key"]) != "${{steps.build.outputs.build-key}}" {
		violations = append(violations, buildOnceViolation(name, "build", "build outputs must bind the exact upload, source SHA, and build key"))
	}

	buildCount := 0
	directBuild := false
	for index := range job.Steps {
		step := &job.Steps[index]
		buildCount += strings.Count(step.Run, buildVerityCommand)
		directBuild = directBuild || strings.Contains(step.Run, "go build ")
	}
	if buildCount != 1 || directBuild {
		violations = append(violations, buildOnceViolation(name, "build", "exactly one Verity build is allowed"))
	}
	if !exactBuildIdentityStep(job.Steps) {
		violations = append(violations, buildOnceViolation(name, "build", "build helper must bind exact source and current run identity"))
	}
	if !exactBuildUpload(job.Steps) {
		violations = append(violations, buildOnceViolation(name, "build", "artifact upload must contain the exact canonical three-file directory"))
	}
	return violations
}

func exactBuildIdentityStep(steps []workflowStep) bool {
	for index := range steps {
		step := &steps[index]
		if step.ID != "build" || !strings.Contains(step.Run, buildVerityCommand) {
			continue
		}
		expectedEnvironment := map[string]string{
			"VERITY_SOURCE_SHA":   "${{ inputs.source_sha }}",
			"VERITY_ARTIFACT_DIR": "${{ runner.temp }}/verity-artifact",
			"VERITY_RUN_ID":       "${{ github.run_id }}",
			"VERITY_RUN_ATTEMPT":  "${{ github.run_attempt }}",
		}
		if len(step.Env) != len(expectedEnvironment) {
			return false
		}
		for key, value := range expectedEnvironment {
			if step.Env[key] != value {
				return false
			}
		}
		for _, marker := range []string{
			`--source-sha "$VERITY_SOURCE_SHA"`, `--run-id "$VERITY_RUN_ID"`,
			`--run-attempt "$VERITY_RUN_ATTEMPT"`, `--artifact-directory "$VERITY_ARTIFACT_DIR"`,
			`--github-output "$GITHUB_OUTPUT"`,
		} {
			if !strings.Contains(step.Run, marker) {
				return false
			}
		}
		return true
	}
	return false
}

func exactBuildUpload(steps []workflowStep) bool {
	for index := range steps {
		step := &steps[index]
		if step.ID != "upload" || actionName(step.Uses) != "actions/upload-artifact" {
			continue
		}
		return normalizeExpression(step.With["name"]) == "${{steps.build.outputs.artifact-name}}" &&
			step.With["path"] == "${{ runner.temp }}/verity-artifact/" &&
			step.With["if-no-files-found"] == "error" && step.With["compression-level"] == "0" &&
			step.With["overwrite"] == "false" && step.With["include-hidden-files"] == "false"
	}
	return false
}

func validateAttestJob(name string, job *workflowJob) []Violation {
	var violations []Violation
	if normalizeExpression(job.If) != protectedAttestationIf {
		violations = append(violations, buildOnceViolation(name, "attest", "protected attestation gate must bind the strict input and exclude forks and unprotected refs"))
	}
	if len(job.Needs) != 1 || job.Needs[0] != "build" {
		violations = append(violations, buildOnceViolation(name, "attest", "attestation must depend only on the completed build"))
	}
	expected := map[permissionScope]permissionLevel{
		actionsScope: permissionRead, contentsScope: permissionRead,
		idTokenScope: permissionWrite, attestationsScope: permissionWrite,
	}
	if !buildOnceExactPermissions(job.Permissions, expected) {
		violations = append(violations, buildOnceViolation(name, "attest", "id-token and attestations writes must exist only on the protected attestation job"))
	}
	if !exactAttestationSteps(job.Steps) {
		violations = append(violations, buildOnceViolation(name, "attest", "attestation must consume and attest the exact built binary"))
	}
	return violations
}

func exactAttestationSteps(steps []workflowStep) bool {
	setupFound := false
	attestFound := false
	for index := range steps {
		step := &steps[index]
		switch actionName(step.Uses) {
		case "./.github/actions/setup-verity":
			setupFound = normalizeExpression(step.With["artifact-name"]) == "${{needs.build.outputs.artifact-name}}" &&
				normalizeExpression(step.With["artifact-digest"]) == "${{needs.build.outputs.artifact-digest}}" &&
				normalizeExpression(step.With["source-sha"]) == "${{inputs.source_sha}}" &&
				normalizeExpression(step.With["build-key"]) == "${{needs.build.outputs.build-key}}" &&
				step.With["verify-attestation"] == "false"
		case "actions/attest-build-provenance":
			attestFound = step.With["subject-path"] == "verity"
		}
	}
	return setupFound && attestFound
}

func buildOnceReadPermissions(value permissions) bool {
	return buildOnceExactPermissions(value, map[permissionScope]permissionLevel{contentsScope: permissionRead})
}

func buildOnceExactPermissions(value permissions, expected map[permissionScope]permissionLevel) bool {
	if !value.declared || value.all != "" || len(value.scopes) != len(expected) {
		return false
	}
	for scope, level := range expected {
		if value.level(scope) != level {
			return false
		}
	}
	return true
}
