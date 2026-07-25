package workflowpolicy

import (
	"bytes"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	setupVerityCommand  = "go run ./.github/actions/setup-verity/cmd/setup-verity"
	setupDownloadPath   = "${{ runner.temp }}/setup-verity/${{ github.run_id }}-${{ github.run_attempt }}-${{ github.job }}/download"
	setupArtifactPath   = "${{ runner.temp }}/setup-verity/${{ github.run_id }}-${{ github.run_attempt }}-${{ github.job }}/artifact"
	setupSignerWorkflow = "github.com/verity-org/verity/.github/workflows/build-verity.yaml"
)

type setupActionInput struct {
	Description string `yaml:"description"`
	Required    bool   `yaml:"required"`
	Default     string `yaml:"default"`
}

type setupAction struct {
	Name        string                      `yaml:"name"`
	Description string                      `yaml:"description"`
	Inputs      map[string]setupActionInput `yaml:"inputs"`
	Outputs     map[string]yaml.Node        `yaml:"outputs"`
	Runs        struct {
		Using string         `yaml:"using"`
		Steps []workflowStep `yaml:"steps"`
	} `yaml:"runs"`
}

func validateSetupVerityAction(name string, data []byte) ([]Violation, error) {
	var action setupAction
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&action); err != nil {
		return nil, fmt.Errorf("decode setup-verity action %q: %w", name, err)
	}

	violations := make([]Violation, 0, 16)
	violations = append(violations, validateSetupActionSurface(name, &action)...)
	violations = append(violations, validateSetupActionSteps(name, action.Runs.Steps)...)
	for index := range action.Runs.Steps {
		violations = append(violations, validateActionReference(name, "composite", action.Runs.Steps[index].Uses)...)
	}
	if strings.Contains(string(data), "${{ secrets.") {
		violations = append(violations, buildOnceViolation(name, "composite", "secret exposure is forbidden in setup-verity"))
	}
	return violations, nil
}

func validateSetupActionSurface(name string, action *setupAction) []Violation {
	var violations []Violation
	if action.Name != "Setup Verity" || action.Runs.Using != "composite" || len(action.Outputs) != 0 {
		violations = append(violations, buildOnceViolation(name, "composite", "setup-verity must be an output-free composite action"))
	}
	if !exactSetupInputs(action.Inputs) {
		violations = append(violations, buildOnceViolation(name, "composite", "setup-verity inputs must be exact and verify-attestation must default false"))
	}
	return violations
}

func exactSetupInputs(inputs map[string]setupActionInput) bool {
	if len(inputs) != 5 {
		return false
	}
	for _, name := range []string{"artifact-name", "artifact-digest", "source-sha", "build-key"} {
		input, exists := inputs[name]
		if !exists || !input.Required || input.Default != "" {
			return false
		}
	}
	verify, exists := inputs["verify-attestation"]
	return exists && !verify.Required && verify.Default == "false"
}

func validateSetupActionSteps(name string, steps []workflowStep) []Violation {
	var violations []Violation
	for index := range steps {
		if strings.HasPrefix(actionName(steps[index].Uses), "actions/cache") {
			violations = append(violations, buildOnceViolation(name, "composite", "no executable cache may be trusted"))
		}
	}
	remoteIndex, remote := findSetupRunStep(steps, setupVerityCommand+" verify-remote ")
	downloadIndex, download := findSetupActionStep(steps, "actions/download-artifact")
	extractIndex, extract := findSetupRunStep(steps, setupVerityCommand+" extract ")
	attestIndex, attest := findSetupRunStep(steps, "gh attestation verify ")
	activateIndex, activate := findSetupRunStep(steps, setupVerityCommand+" activate ")

	violations = append(violations, validateRemoteStep(name, remote)...)
	violations = append(violations, validateDownloadStep(name, download)...)
	violations = append(violations, validateExtractStep(name, extract)...)
	violations = append(violations, validateAttestationStep(name, attest)...)
	violations = append(violations, validateActivationStep(name, activate)...)
	if remoteIndex < 0 || downloadIndex <= remoteIndex || extractIndex <= downloadIndex || attestIndex <= extractIndex || activateIndex <= attestIndex {
		violations = append(violations, buildOnceViolation(name, "composite", "identity, download, checksum, attestation, and activation steps must remain ordered"))
	}
	return violations
}

func validateRemoteStep(name string, step *workflowStep) []Violation {
	if step == nil {
		return []Violation{buildOnceViolation(name, "composite", "checksum and remote identity verification must use the trusted helper")}
	}
	checks := []struct {
		key    string
		value  string
		detail string
	}{
		{key: "VERITY_ARTIFACT_NAME", value: "${{ inputs.artifact-name }}", detail: "artifact name must bind the exact action input"},
		{key: "VERITY_ARTIFACT_DIGEST", value: "${{ inputs.artifact-digest }}", detail: "artifact digest must bind the exact action input"},
		{key: "VERITY_SOURCE_SHA", value: "${{ inputs.source-sha }}", detail: "source SHA must bind the exact action input"},
		{key: "VERITY_BUILD_KEY", value: "${{ inputs.build-key }}", detail: "build key must bind the exact action input"},
		{key: "VERITY_PROTECTED", value: "${{ inputs.verify-attestation }}", detail: "protected mode must bind the exact action input"},
	}
	var violations []Violation
	if step.ID != "verify-remote" {
		violations = append(violations, buildOnceViolation(name, "composite", "remote verification must expose the exact artifact ID and normalized attestation mode"))
	}
	for _, check := range checks {
		if step.Env[check.key] != check.value {
			violations = append(violations, buildOnceViolation(name, "composite", check.detail))
		}
	}
	for _, marker := range []string{
		`--artifact-name "$VERITY_ARTIFACT_NAME"`, `--artifact-digest "$VERITY_ARTIFACT_DIGEST"`,
		`--source-sha "$VERITY_SOURCE_SHA"`, `--build-key "$VERITY_BUILD_KEY"`,
		`--repository "$GITHUB_REPOSITORY"`, `--run-id "$GITHUB_RUN_ID"`,
		`--run-attempt "$GITHUB_RUN_ATTEMPT"`,
		`--protected-attestation "$VERITY_PROTECTED"`, `--github-output "$GITHUB_OUTPUT"`,
	} {
		if !strings.Contains(step.Run, marker) {
			violations = append(violations, buildOnceViolation(name, "composite", "remote identity command must bind name, digest, source SHA, build key, repository, and current run attempt"))
			break
		}
	}
	return violations
}

func validateDownloadStep(name string, step *workflowStep) []Violation {
	if step == nil {
		return []Violation{buildOnceViolation(name, "composite", "exact current-run artifact download is required")}
	}
	var violations []Violation
	if len(step.With) != 4 || normalizeExpression(step.With["artifact-ids"]) != "${{steps.verify-remote.outputs.artifact-id}}" {
		violations = append(violations, buildOnceViolation(name, "composite", "download must select the verified current-run artifact ID"))
	}
	if step.With["path"] != setupDownloadPath {
		violations = append(violations, buildOnceViolation(name, "composite", "download must use an immutable runner temp path"))
	}
	if step.With["skip-decompress"] != "true" || step.With["digest-mismatch"] != "error" {
		violations = append(violations, buildOnceViolation(name, "composite", "automatic archive extraction is forbidden and digest mismatch must fail"))
	}
	return violations
}

func validateExtractStep(name string, step *workflowStep) []Violation {
	if step == nil {
		return []Violation{buildOnceViolation(name, "composite", "checksum verification must use the trusted exact archive extractor")}
	}
	checks := map[string]string{
		"VERITY_DOWNLOAD_DIR": setupDownloadPath, "VERITY_ARTIFACT_DIR": setupArtifactPath,
		"VERITY_ARTIFACT_DIGEST": "${{ inputs.artifact-digest }}", "VERITY_SOURCE_SHA": "${{ inputs.source-sha }}",
		"VERITY_BUILD_KEY": "${{ inputs.build-key }}",
	}
	for key, value := range checks {
		if step.Env[key] != value {
			return []Violation{buildOnceViolation(name, "composite", "checksum, source SHA, build key, and immutable paths must bind exact inputs")}
		}
	}
	return nil
}

func validateAttestationStep(name string, step *workflowStep) []Violation {
	if step == nil {
		return []Violation{buildOnceViolation(name, "composite", "protected signer workflow verification is required")}
	}
	var violations []Violation
	if normalizeExpression(step.If) != "${{steps.verify-remote.outputs.verify-attestation=='true'}}" {
		violations = append(violations, buildOnceViolation(name, "composite", "attestation verification must use the helper-normalized strict boolean"))
	}
	markers := []string{
		`--repo "$GITHUB_REPOSITORY"`, `--signer-repo "verity-org/verity"`,
		`--signer-workflow "` + setupSignerWorkflow + `"`,
		`--signer-digest "$VERITY_SOURCE_SHA"`, `--source-digest "$VERITY_SOURCE_SHA"`, `--deny-self-hosted-runners`,
		`--predicate-type "https://slsa.dev/provenance/v1"`,
	}
	for _, marker := range markers {
		if !strings.Contains(step.Run, marker) {
			violations = append(violations, buildOnceViolation(name, "composite", "signer workflow, repository, source SHA, and binary digest must be verified"))
			break
		}
	}
	return violations
}

func validateActivationStep(name string, step *workflowStep) []Violation {
	if step == nil {
		return []Violation{buildOnceViolation(name, "composite", "symlink activation is forbidden; trusted activation must reverify before chmod")}
	}
	checks := map[string]string{
		"VERITY_ARTIFACT_DIR": setupArtifactPath,
		"VERITY_SOURCE_SHA":   "${{ inputs.source-sha }}",
		"VERITY_BUILD_KEY":    "${{ inputs.build-key }}",
	}
	for key, value := range checks {
		if step.Env[key] != value {
			return []Violation{buildOnceViolation(name, "composite", "activation must bind the exact source SHA and build key")}
		}
	}
	if !strings.Contains(step.Run, `--destination "$GITHUB_WORKSPACE/verity"`) {
		return []Violation{buildOnceViolation(name, "composite", "activation destination must be the workspace Verity binary")}
	}
	return nil
}

func findSetupRunStep(steps []workflowStep, marker string) (int, *workflowStep) {
	for index := range steps {
		if strings.Contains(steps[index].Run, marker) {
			return index, &steps[index]
		}
	}
	return -1, nil
}

func findSetupActionStep(steps []workflowStep, target string) (int, *workflowStep) {
	for index := range steps {
		if actionName(steps[index].Uses) == target {
			return index, &steps[index]
		}
	}
	return -1, nil
}
