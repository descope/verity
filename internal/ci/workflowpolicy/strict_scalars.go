package workflowpolicy

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

func validateString(node *yaml.Node, path string) error {
	if node.Kind != yaml.ScalarNode || node.Tag != "!!str" {
		return fmt.Errorf("%w: %s: expected string scalar", errStrictYAMLSchema, path)
	}
	return nil
}

func validateBoolean(node *yaml.Node, path string) error {
	if node.Kind != yaml.ScalarNode || node.Tag != "!!bool" {
		return fmt.Errorf("%w: %s: expected boolean scalar", errStrictYAMLSchema, path)
	}
	return nil
}

func validateNumber(node *yaml.Node, path string) error {
	if node.Kind != yaml.ScalarNode || (node.Tag != "!!int" && node.Tag != "!!float") {
		return fmt.Errorf("%w: %s: expected numeric scalar", errStrictYAMLSchema, path)
	}
	return nil
}

func validateBooleanOrExpression(node *yaml.Node, path string) error {
	if node.Kind == yaml.ScalarNode && node.Tag == "!!bool" {
		return nil
	}
	return validateExpression(node, path, "boolean")
}

func validateIntegerOrExpression(node *yaml.Node, path string) error {
	if node.Kind == yaml.ScalarNode && node.Tag == "!!int" {
		return nil
	}
	return validateExpression(node, path, "integer")
}

func validateExpression(node *yaml.Node, path, expected string) error {
	if err := validateString(node, path); err != nil {
		return err
	}
	value := strings.TrimSpace(node.Value)
	if !strings.HasPrefix(value, "${{") || !strings.HasSuffix(value, "}}") {
		return fmt.Errorf("%w: %s: expected %s or GitHub expression", errStrictYAMLSchema, path, expected)
	}
	return nil
}

func validateStringOrInteger(node *yaml.Node, path string) error {
	if node.Kind == yaml.ScalarNode && (node.Tag == "!!str" || node.Tag == "!!int") {
		return nil
	}
	return fmt.Errorf("%w: %s: expected string or integer scalar", errStrictYAMLSchema, path)
}

func validateStringOrIntegerSequence(node *yaml.Node, path string) error {
	if node.Kind != yaml.SequenceNode {
		return fmt.Errorf("%w: %s: expected string/integer sequence", errStrictYAMLSchema, path)
	}
	for index, child := range node.Content {
		if err := validateStringOrInteger(child, fmt.Sprintf("%s[%d]", path, index)); err != nil {
			return err
		}
	}
	return nil
}

func validateInputScalar(node *yaml.Node, path string) error {
	if node.Kind != yaml.ScalarNode {
		return fmt.Errorf("%w: %s: expected input scalar", errStrictYAMLSchema, path)
	}
	switch node.Tag {
	case "!!str", "!!bool", "!!int", "!!float":
		return nil
	default:
		return fmt.Errorf("%w: %s: unsupported input scalar type %q", errStrictYAMLSchema, path, node.Tag)
	}
}

func validateMatrixValue(node *yaml.Node, path string) error {
	switch node.Kind {
	case yaml.ScalarNode:
		switch node.Tag {
		case "!!str", "!!bool", "!!int", "!!float":
			return nil
		default:
			return fmt.Errorf("%w: %s: unsupported matrix scalar type %q", errStrictYAMLSchema, path, node.Tag)
		}
	case yaml.SequenceNode:
		return validateSequence(validateMatrixValue)(node, path)
	case yaml.MappingNode:
		return validateMapping(node, path, yamlMappingSchema{extension: validateMatrixValue})
	default:
		return fmt.Errorf("%w: %s: invalid matrix value", errStrictYAMLSchema, path)
	}
}
