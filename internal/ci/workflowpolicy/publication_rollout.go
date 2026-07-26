package workflowpolicy

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const RulePublicationRollout Rule = "publication-rollout"

var errMultipleSignerRolloutValues = errors.New("multiple signer rollout JSON values")

type signerRolloutState struct {
	Image     string `json:"image"`
	Digest    string `json:"digest"`
	Workflow  string `json:"workflow"`
	SourceSHA string `json:"source_sha"`
	Bootstrap bool   `json:"bootstrap"`
	Runnable  bool   `json:"runnable"`
}

func validatePublicationRollout(directory string, workflows []workflowFile) []Violation {
	root := filepath.Dir(filepath.Dir(filepath.Clean(directory)))
	data, err := os.ReadFile(filepath.Join(root, "ci", "apk-signer.lock.json"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return []Violation{publicationRolloutViolation("read signer lock: " + err.Error())}
	}
	state, err := decodeSignerRollout(data)
	if err != nil {
		return []Violation{publicationRolloutViolation(err.Error())}
	}
	publish, exists := findWorkflow(workflows, "publish.yaml")
	if !exists {
		return []Violation{publicationRolloutViolation("publish workflow is missing")}
	}
	if state.Bootstrap {
		return validateBootstrapPublicationTriggers(&publish, state)
	}
	return validateRunnablePublicationTriggers(&publish, state)
}

func decodeSignerRollout(data []byte) (signerRolloutState, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var state signerRolloutState
	if err := decoder.Decode(&state); err != nil {
		return signerRolloutState{}, fmt.Errorf("decode signer rollout: %w", err)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return signerRolloutState{}, fmt.Errorf("decode signer rollout: %w", errMultipleSignerRolloutValues)
		}
		return signerRolloutState{}, fmt.Errorf("decode signer rollout trailing data: %w", err)
	}
	return state, nil
}

func validateBootstrapPublicationTriggers(file *workflowFile, state signerRolloutState) []Violation {
	if state.Runnable || state.Digest != "" || state.SourceSHA != "" {
		return []Violation{publicationRolloutViolation("bootstrap lock must be non-runnable with empty immutable coordinates")}
	}
	triggers := file.Workflow.On
	if !triggers.WorkflowDispatch || triggers.Push.Present || triggers.Schedule || triggers.WorkflowCall ||
		triggers.PullRequest || triggers.PullRequestTarget || triggers.WorkflowRun || triggers.OtherEvent {
		return []Violation{publicationRolloutViolation("bootstrap lock requires a manual-only publish workflow")}
	}
	return nil
}

func validateRunnablePublicationTriggers(file *workflowFile, state signerRolloutState) []Violation {
	if !state.Runnable || state.Digest == "" || state.SourceSHA == "" {
		return []Violation{publicationRolloutViolation("automatic publication requires a runnable immutable signer lock")}
	}
	triggers := file.Workflow.On
	if !triggers.WorkflowDispatch || !triggers.Push.Present || !triggers.Schedule || triggers.WorkflowCall ||
		triggers.PullRequest || triggers.PullRequestTarget || triggers.WorkflowRun || triggers.OtherEvent {
		return []Violation{publicationRolloutViolation("runnable signer lock requires push, schedule, and manual publication triggers only")}
	}
	return nil
}

func publicationRolloutViolation(detail string) Violation {
	return Violation{Rule: RulePublicationRollout, Workflow: "publish.yaml", Detail: detail}
}
