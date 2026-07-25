package workflowpolicy

import (
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

var secretNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*$`)

func validateSecretsNode(node *yaml.Node, path string) error {
	switch node.Kind {
	case yaml.ScalarNode:
		if err := validateString(node, path); err != nil {
			return err
		}
		if node.Value != "inherit" {
			return fmt.Errorf("%w: %s: scalar secrets must be the exact inherit literal", errStrictYAMLSchema, path)
		}
		return nil
	case yaml.MappingNode:
		if len(node.Content) == 0 {
			return fmt.Errorf("%w: %s: secrets mapping must not be empty", errStrictYAMLSchema, path)
		}
		seen := make(map[string]struct{}, len(node.Content)/2)
		for index := 0; index < len(node.Content); index += 2 {
			key := node.Content[index]
			value := node.Content[index+1]
			if !secretNamePattern.MatchString(key.Value) {
				return fmt.Errorf("%w: %s: invalid secret name %q", errStrictYAMLSchema, path, key.Value)
			}
			canonical := strings.ToUpper(key.Value)
			if _, duplicate := seen[canonical]; duplicate {
				return fmt.Errorf("%w: %s: duplicate secret name %q", errStrictYAMLSchema, path, key.Value)
			}
			seen[canonical] = struct{}{}
			if err := validateString(value, childPath(path, key.Value)); err != nil {
				return err
			}
			if strings.TrimSpace(value.Value) == "" {
				return fmt.Errorf("%w: %s: secret value must not be empty", errStrictYAMLSchema, childPath(path, key.Value))
			}
		}
		return nil
	default:
		return fmt.Errorf("%w: %s: secrets must be inherit or a mapping", errStrictYAMLSchema, path)
	}
}

func validateJobSecretsPlacement(node *yaml.Node, path string) error {
	hasSecrets := false
	hasUses := false
	hasRunFields := false
	for index := 0; index < len(node.Content); index += 2 {
		switch node.Content[index].Value {
		case "secrets":
			hasSecrets = true
		case "uses":
			hasUses = true
		case "runs-on", "environment", "outputs", "env", "defaults", "steps", "timeout-minutes",
			"continue-on-error", "container", "services":
			hasRunFields = true
		}
	}
	if hasSecrets && (!hasUses || hasRunFields) {
		return fmt.Errorf("%w: %s: secrets is only valid on reusable workflow jobs", errStrictYAMLSchema, path)
	}
	return nil
}
