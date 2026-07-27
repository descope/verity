package workflowpolicy

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

var triggerEventsWithTypes = map[string]struct{}{
	"branch_protection_rule":      {},
	"check_run":                   {},
	"check_suite":                 {},
	"discussion":                  {},
	"discussion_comment":          {},
	"issue_comment":               {},
	"issues":                      {},
	"label":                       {},
	"milestone":                   {},
	"project":                     {},
	"project_card":                {},
	"project_column":              {},
	"pull_request_review":         {},
	"pull_request_review_comment": {},
	"registry_package":            {},
	"release":                     {},
	"watch":                       {},
}

type workflowInputMetadata struct {
	typeName     string
	defaultNode  *yaml.Node
	typeDeclared bool
}

func validateTriggersNode(node *yaml.Node, path string) error {
	switch node.Kind {
	case yaml.ScalarNode:
		if err := validateString(node, path); err != nil {
			return err
		}
		return validateTriggerName(node.Value, path)
	case yaml.SequenceNode:
		for index, event := range node.Content {
			eventPath := fmt.Sprintf("%s[%d]", path, index)
			if err := validateString(event, eventPath); err != nil {
				return err
			}
			if err := validateTriggerName(event.Value, eventPath); err != nil {
				return err
			}
		}
		return nil
	case yaml.MappingNode:
		for index := 0; index < len(node.Content); index += 2 {
			name := node.Content[index].Value
			if err := validateTriggerName(name, childPath(path, name)); err != nil {
				return err
			}
			if err := validateTriggerConfig(name, node.Content[index+1], childPath(path, name)); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("%s: %w", path, errTriggerShape)
	}
}

func validateTriggerName(name, path string) error {
	if name != strings.TrimSpace(name) || name != strings.ToLower(name) {
		return fmt.Errorf("%s: %w %q", path, errNonCanonicalTrigger, name)
	}
	if _, known := knownWorkflowEvents[name]; !known {
		return fmt.Errorf("%s: %w %q", path, errUnsupportedTrigger, name)
	}
	return nil
}

func validateTriggerConfig(name string, node *yaml.Node, path string) error {
	if node.Kind == yaml.ScalarNode && node.Tag == "!!null" {
		return nil
	}
	switch name {
	case "push":
		return validatePushConfig(node, path)
	case "pull_request", "pull_request_target":
		return validatePullRequestConfig(node, path)
	case "workflow_call":
		return validateWorkflowCallConfig(node, path)
	case "workflow_dispatch":
		return validateWorkflowDispatchConfig(node, path)
	case "workflow_run":
		return validateWorkflowRunConfig(node, path)
	case "schedule":
		return validateScheduleConfig(node, path)
	case "repository_dispatch":
		return validateMapping(node, path, yamlMappingSchema{fields: map[string]yamlValidator{
			"types": validateStringSequence,
		}})
	case "merge_group":
		return validateMapping(node, path, yamlMappingSchema{fields: map[string]yamlValidator{
			"types":           validateStringSequence,
			"branches":        validateStringSequence,
			"branches-ignore": validateStringSequence,
		}})
	default:
		if _, supportsTypes := triggerEventsWithTypes[name]; !supportsTypes {
			return validateMapping(node, path, yamlMappingSchema{})
		}
		return validateMapping(node, path, yamlMappingSchema{fields: map[string]yamlValidator{
			"types": validateStringSequence,
		}})
	}
}

func validatePushConfig(node *yaml.Node, path string) error {
	return validateMapping(node, path, yamlMappingSchema{fields: map[string]yamlValidator{
		"branches":        validateStringSequence,
		"branches-ignore": validateStringSequence,
		"paths":           validateStringSequence,
		"paths-ignore":    validateStringSequence,
		"tags":            validateStringSequence,
		"tags-ignore":     validateStringSequence,
	}})
}

func validatePullRequestConfig(node *yaml.Node, path string) error {
	return validateMapping(node, path, yamlMappingSchema{fields: map[string]yamlValidator{
		"branches":        validateStringSequence,
		"branches-ignore": validateStringSequence,
		"paths":           validateStringSequence,
		"paths-ignore":    validateStringSequence,
		"types":           validateStringSequence,
	}})
}

func validateWorkflowCallConfig(node *yaml.Node, path string) error {
	return validateMapping(node, path, yamlMappingSchema{fields: map[string]yamlValidator{
		"inputs":  validateWorkflowCallInputs,
		"outputs": validateWorkflowCallOutputs,
		"secrets": validateWorkflowCallSecrets,
	}})
}

func validateWorkflowCallInputs(node *yaml.Node, path string) error {
	return validateMapping(node, path, yamlMappingSchema{extension: validateWorkflowCallInputNode})
}

func validateWorkflowCallOutputs(node *yaml.Node, path string) error {
	return validateMapping(node, path, yamlMappingSchema{extension: func(node *yaml.Node, path string) error {
		return validateMapping(node, path, yamlMappingSchema{fields: map[string]yamlValidator{
			"description": validateString,
			"value":       validateString,
		}})
	}})
}

func validateWorkflowCallSecrets(node *yaml.Node, path string) error {
	return validateMapping(node, path, yamlMappingSchema{extension: func(node *yaml.Node, path string) error {
		return validateMapping(node, path, yamlMappingSchema{fields: map[string]yamlValidator{
			"description": validateString,
			"required":    validateBoolean,
		}})
	}})
}

func validateWorkflowDispatchConfig(node *yaml.Node, path string) error {
	return validateMapping(node, path, yamlMappingSchema{fields: map[string]yamlValidator{
		"inputs": validateWorkflowDispatchInputs,
	}})
}

func validateWorkflowRunConfig(node *yaml.Node, path string) error {
	return validateMapping(node, path, yamlMappingSchema{fields: map[string]yamlValidator{
		"workflows":       validateStringSequence,
		"types":           validateStringSequence,
		"branches":        validateStringSequence,
		"branches-ignore": validateStringSequence,
	}})
}

func validateScheduleConfig(node *yaml.Node, path string) error {
	return validateSequence(func(node *yaml.Node, path string) error {
		return validateMapping(node, path, yamlMappingSchema{fields: map[string]yamlValidator{
			"cron": validateString,
		}})
	})(node, path)
}

func validateWorkflowCallInputNode(node *yaml.Node, path string) error {
	return validateWorkflowInputNode(node, path, false)
}

func validateWorkflowDispatchInputs(node *yaml.Node, path string) error {
	return validateMapping(node, path, yamlMappingSchema{extension: func(node *yaml.Node, path string) error {
		return validateWorkflowInputNode(node, path, true)
	}})
}

func validateWorkflowInputNode(node *yaml.Node, path string, dispatch bool) error {
	fields := map[string]yamlValidator{
		"description": validateString,
		"required":    validateBoolean,
		"default":     validateInputScalar,
		"type":        validateString,
	}
	if dispatch {
		fields["options"] = validateStringSequence
	}
	if err := validateMapping(node, path, yamlMappingSchema{fields: fields}); err != nil {
		return err
	}

	metadata := readWorkflowInputMetadata(node)
	if !dispatch && !metadata.typeDeclared {
		return fmt.Errorf("%w: %s: workflow_call input type is required", errStrictYAMLSchema, path)
	}
	if err := validateInputType(metadata.typeName, dispatch, path); err != nil {
		return err
	}
	if metadata.defaultNode == nil {
		return nil
	}
	switch metadata.typeName {
	case "boolean":
		return validateBoolean(metadata.defaultNode, childPath(path, "default"))
	case "number":
		return validateNumber(metadata.defaultNode, childPath(path, "default"))
	default:
		return validateString(metadata.defaultNode, childPath(path, "default"))
	}
}

func readWorkflowInputMetadata(node *yaml.Node) workflowInputMetadata {
	metadata := workflowInputMetadata{typeName: "string"}
	for index := 0; index < len(node.Content); index += 2 {
		switch node.Content[index].Value {
		case "type":
			metadata.typeName = node.Content[index+1].Value
			metadata.typeDeclared = true
		case "default":
			metadata.defaultNode = node.Content[index+1]
		}
	}
	return metadata
}

func validateInputType(typeName string, dispatch bool, path string) error {
	switch typeName {
	case "boolean", "number", "string":
		return nil
	case "choice", "environment":
		if dispatch {
			return nil
		}
	}
	return fmt.Errorf("%w: %s: unsupported input type %q", errStrictYAMLSchema, path, typeName)
}
