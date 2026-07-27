package workflowpolicy

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

type yamlValidator func(*yaml.Node, string) error

type yamlMappingSchema struct {
	fields    map[string]yamlValidator
	extension yamlValidator
}

var allowedYAMLTags = map[string]struct{}{
	"":            {},
	"!!map":       {},
	"!!seq":       {},
	"!!str":       {},
	"!!bool":      {},
	"!!int":       {},
	"!!float":     {},
	"!!null":      {},
	"!!timestamp": {},
	"!!binary":    {},
}

func validateStrictWorkflowYAML(document *yaml.Node) error {
	if document.Kind != yaml.DocumentNode || len(document.Content) != 1 {
		return fmt.Errorf("workflow document: %w", errExpectedMapping)
	}
	root := document.Content[0]
	if err := validateYAMLGraph(root, "workflow"); err != nil {
		return err
	}
	return validateWorkflowNode(root, "workflow")
}

func validateYAMLGraph(node *yaml.Node, path string) error {
	if err := validateYAMLNodeHeader(node, path); err != nil {
		return err
	}

	switch node.Kind {
	case yaml.MappingNode:
		seen := make(map[string]struct{}, len(node.Content)/2)
		for index := 0; index < len(node.Content); index += 2 {
			key := node.Content[index]
			value := node.Content[index+1]
			if err := validateYAMLGraph(key, path+".<key>"); err != nil {
				return err
			}
			if key.Kind != yaml.ScalarNode || key.Tag != "!!str" {
				return fmt.Errorf("%w: %s: mapping keys must be strings", errStrictYAMLSchema, path)
			}
			if key.Value == "<<" {
				return fmt.Errorf("%w: %s: YAML merge keys are not allowed", errStrictYAMLSchema, path)
			}
			if _, duplicate := seen[key.Value]; duplicate {
				return fmt.Errorf("%w: %s: duplicate key %q", errStrictYAMLSchema, path, key.Value)
			}
			seen[key.Value] = struct{}{}
			if err := validateYAMLGraph(value, childPath(path, key.Value)); err != nil {
				return err
			}
		}
	case yaml.SequenceNode:
		for index, child := range node.Content {
			if err := validateYAMLGraph(child, fmt.Sprintf("%s[%d]", path, index)); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateYAMLNodeHeader(node *yaml.Node, path string) error {
	if node.Anchor != "" {
		return fmt.Errorf("%w: %s: YAML anchors are not allowed", errStrictYAMLSchema, path)
	}
	if node.Kind == yaml.AliasNode {
		return fmt.Errorf("%w: %s: YAML aliases are not allowed", errStrictYAMLSchema, path)
	}
	if node.Style&yaml.TaggedStyle != 0 {
		return fmt.Errorf("%w: %s: explicit YAML tags are not allowed", errStrictYAMLSchema, path)
	}
	if _, allowed := allowedYAMLTags[node.Tag]; !allowed {
		return fmt.Errorf("%w: %s: custom YAML tag %q is not allowed", errStrictYAMLSchema, path, node.Tag)
	}
	return nil
}

func validateMapping(node *yaml.Node, path string, schema yamlMappingSchema) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("%s: %w", path, errExpectedMapping)
	}
	for index := 0; index < len(node.Content); index += 2 {
		key := node.Content[index].Value
		value := node.Content[index+1]
		validator, known := schema.fields[key]
		if !known {
			validator = schema.extension
		}
		if validator == nil {
			return fmt.Errorf("%w: %s: unknown field %q", errStrictYAMLSchema, path, key)
		}
		if err := validator(value, childPath(path, key)); err != nil {
			return err
		}
	}
	return nil
}

func validateStringSequence(node *yaml.Node, path string) error {
	if node.Kind == yaml.ScalarNode {
		return validateString(node, path)
	}
	if node.Kind != yaml.SequenceNode {
		return fmt.Errorf("%w: %s: expected string or string sequence", errStrictYAMLSchema, path)
	}
	for index, child := range node.Content {
		if err := validateString(child, fmt.Sprintf("%s[%d]", path, index)); err != nil {
			return err
		}
	}
	return nil
}

func validateSequence(validator yamlValidator) yamlValidator {
	return func(node *yaml.Node, path string) error {
		if node.Kind != yaml.SequenceNode {
			return fmt.Errorf("%w: %s: expected YAML sequence", errStrictYAMLSchema, path)
		}
		for index, child := range node.Content {
			if err := validator(child, fmt.Sprintf("%s[%d]", path, index)); err != nil {
				return err
			}
		}
		return nil
	}
}

func validateExtensionScalars(node *yaml.Node, path string) error {
	return validateMapping(node, path, yamlMappingSchema{extension: validateString})
}

func validateInputScalars(node *yaml.Node, path string) error {
	return validateMapping(node, path, yamlMappingSchema{extension: validateInputScalar})
}

func childPath(path, key string) string {
	return path + "." + key
}
