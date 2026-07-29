package workflowpolicy

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

var knownWorkflowEvents = map[string]struct{}{
	"branch_protection_rule": {}, "check_run": {}, "check_suite": {}, "create": {},
	"delete": {}, "deployment": {}, "deployment_status": {}, "discussion": {},
	"discussion_comment": {}, "fork": {}, "gollum": {}, "issue_comment": {},
	"issues": {}, "label": {}, "merge_group": {}, "milestone": {}, "page_build": {},
	"project": {}, "project_card": {}, "project_column": {}, "public": {},
	"pull_request": {}, "pull_request_review": {}, "pull_request_review_comment": {},
	"pull_request_target": {}, "push": {}, "registry_package": {}, "release": {},
	"repository_dispatch": {}, "schedule": {}, "status": {}, "watch": {},
	"workflow_call": {}, "workflow_dispatch": {}, "workflow_run": {},
}

type triggers struct {
	Push              pushTrigger
	PullRequest       bool
	PullRequestTarget bool
	Schedule          bool
	WorkflowCall      bool
	WorkflowDispatch  bool
	WorkflowRun       bool
	OtherEvent        bool
	WorkflowInputs    map[string]workflowCallInput
	WorkflowOutputs   map[string]workflowCallOutput
	WorkflowSecrets   map[string]workflowCallSecret
	DispatchInputs    map[string]workflowDispatchInput
}

type pushTrigger struct {
	Present bool
	Paths   []string
}

type workflowCallInput struct {
	Required bool   `yaml:"required"`
	Type     string `yaml:"type"`
}

type workflowCallOutput struct {
	Value string `yaml:"value"`
}

type workflowCallSecret struct {
	Required bool `yaml:"required"`
}

type workflowDispatchInput struct {
	Required bool        `yaml:"required"`
	Type     string      `yaml:"type"`
	Default  scalarValue `yaml:"default"`
}

func (t *triggers) UnmarshalYAML(node *yaml.Node) error {
	*t = triggers{}
	switch node.Kind {
	case yaml.ScalarNode:
		return t.addEvent(node.Value, nil)
	case yaml.SequenceNode:
		for _, event := range node.Content {
			if event.Kind != yaml.ScalarNode {
				return errTriggerListScalars
			}
			if err := t.addEvent(event.Value, nil); err != nil {
				return err
			}
		}
		return nil
	case yaml.MappingNode:
		for index := 0; index < len(node.Content); index += 2 {
			if err := t.addEvent(node.Content[index].Value, node.Content[index+1]); err != nil {
				return err
			}
		}
		return nil
	default:
		return errTriggerShape
	}
}

func (t *triggers) addEvent(name string, config *yaml.Node) error {
	rawName := name
	name = strings.TrimSpace(name)
	if rawName != name || name != strings.ToLower(name) {
		return fmt.Errorf("%w %q", errNonCanonicalTrigger, name)
	}
	if _, supported := knownWorkflowEvents[name]; !supported {
		return fmt.Errorf("%w %q", errUnsupportedTrigger, name)
	}

	switch name {
	case "push":
		return t.addPush(config)
	case "pull_request":
		t.PullRequest = true
	case "pull_request_target":
		t.PullRequestTarget = true
	case "schedule":
		t.Schedule = true
	case "workflow_call":
		return t.addWorkflowCall(config)
	case "workflow_dispatch":
		return t.addWorkflowDispatch(config)
	case "workflow_run":
		t.WorkflowRun = true
	default:
		t.OtherEvent = true
	}
	return nil
}

func (t *triggers) addWorkflowDispatch(config *yaml.Node) error {
	t.WorkflowDispatch = true
	if config == nil || config.Tag == "!!null" {
		return nil
	}
	var dispatch struct {
		Inputs map[string]workflowDispatchInput `yaml:"inputs"`
	}
	if err := config.Decode(&dispatch); err != nil {
		return fmt.Errorf("decode workflow_dispatch inputs: %w", err)
	}
	t.DispatchInputs = dispatch.Inputs
	return nil
}

func (t *triggers) addPush(config *yaml.Node) error {
	t.Push.Present = true
	if config == nil || config.Tag == "!!null" {
		return nil
	}
	var push struct {
		Paths []string `yaml:"paths"`
	}
	if err := config.Decode(&push); err != nil {
		return fmt.Errorf("decode push trigger: %w", err)
	}
	t.Push.Paths = push.Paths
	return nil
}

func (t *triggers) addWorkflowCall(config *yaml.Node) error {
	t.WorkflowCall = true
	if config == nil || config.Tag == "!!null" {
		return nil
	}
	var call struct {
		Inputs  map[string]workflowCallInput  `yaml:"inputs"`
		Outputs map[string]workflowCallOutput `yaml:"outputs"`
		Secrets map[string]workflowCallSecret `yaml:"secrets"`
	}
	if err := config.Decode(&call); err != nil {
		return fmt.Errorf("decode workflow_call trigger: %w", err)
	}
	t.WorkflowInputs = call.Inputs
	t.WorkflowOutputs = call.Outputs
	t.WorkflowSecrets = call.Secrets
	return nil
}
