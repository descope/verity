package workflowpolicy

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

var permissionFields = map[string]yamlValidator{
	"actions":           validateString,
	"attestations":      validateString,
	"checks":            validateString,
	"contents":          validateString,
	"deployments":       validateString,
	"discussions":       validateString,
	"id-token":          validateString,
	"issues":            validateString,
	"models":            validateString,
	"packages":          validateString,
	"pages":             validateString,
	"pull-requests":     validateString,
	"security-events":   validateString,
	"statuses":          validateString,
	"artifact-metadata": validateString,
}

func validateWorkflowNode(node *yaml.Node, path string) error {
	return validateMapping(node, path, yamlMappingSchema{fields: map[string]yamlValidator{
		"name":        validateString,
		"run-name":    validateString,
		"on":          validateTriggersNode,
		"permissions": validatePermissionsNode,
		"env":         validateExtensionScalars,
		"defaults":    validateDefaultsNode,
		"concurrency": validateConcurrencyNode,
		"jobs":        validateJobsNode,
	}})
}

func validateJobsNode(node *yaml.Node, path string) error {
	return validateMapping(node, path, yamlMappingSchema{extension: validateJobNode})
}

func validateJobNode(node *yaml.Node, path string) error {
	if err := validateMapping(node, path, yamlMappingSchema{fields: map[string]yamlValidator{
		"name":              validateString,
		"permissions":       validatePermissionsNode,
		"needs":             validateStringSequence,
		"if":                validateString,
		"runs-on":           validateRunsOnNode,
		"environment":       validateEnvironmentNode,
		"concurrency":       validateConcurrencyNode,
		"outputs":           validateExtensionScalars,
		"env":               validateExtensionScalars,
		"defaults":          validateDefaultsNode,
		"steps":             validateSequence(validateStepNode),
		"timeout-minutes":   validateIntegerOrExpression,
		"continue-on-error": validateBooleanOrExpression,
		"container":         validateContainerNode,
		"services":          validateServicesNode,
		"strategy":          validateStrategyNode,
		"uses":              validateString,
		"with":              validateInputScalars,
		"secrets":           validateSecretsNode,
	}}); err != nil {
		return err
	}
	return validateJobSecretsPlacement(node, path)
}

func validateStepNode(node *yaml.Node, path string) error {
	return validateMapping(node, path, yamlMappingSchema{fields: map[string]yamlValidator{
		"id":                validateString,
		"if":                validateString,
		"name":              validateString,
		"uses":              validateString,
		"run":               validateString,
		"working-directory": validateString,
		"shell":             validateString,
		"with":              validateExtensionScalars,
		"env":               validateExtensionScalars,
		"continue-on-error": validateBooleanOrExpression,
		"timeout-minutes":   validateIntegerOrExpression,
	}})
}

func validatePermissionsNode(node *yaml.Node, path string) error {
	if node.Kind == yaml.ScalarNode {
		return validateString(node, path)
	}
	return validateMapping(node, path, yamlMappingSchema{fields: permissionFields})
}

func validateDefaultsNode(node *yaml.Node, path string) error {
	return validateMapping(node, path, yamlMappingSchema{fields: map[string]yamlValidator{
		"run": func(node *yaml.Node, path string) error {
			return validateMapping(node, path, yamlMappingSchema{fields: map[string]yamlValidator{
				"shell":             validateString,
				"working-directory": validateString,
			}})
		},
	}})
}

func validateConcurrencyNode(node *yaml.Node, path string) error {
	if node.Kind == yaml.ScalarNode {
		return validateString(node, path)
	}
	return validateMapping(node, path, yamlMappingSchema{fields: map[string]yamlValidator{
		"group":              validateString,
		"cancel-in-progress": validateBooleanOrExpression,
	}})
}

func validateRunsOnNode(node *yaml.Node, path string) error {
	if node.Kind == yaml.MappingNode {
		return validateMapping(node, path, yamlMappingSchema{fields: map[string]yamlValidator{
			"group":  validateString,
			"labels": validateStringSequence,
		}})
	}
	return validateStringSequence(node, path)
}

func validateEnvironmentNode(node *yaml.Node, path string) error {
	if node.Kind == yaml.ScalarNode {
		return validateString(node, path)
	}
	return validateMapping(node, path, yamlMappingSchema{fields: map[string]yamlValidator{
		"name": validateString,
		"url":  validateString,
	}})
}

func validateContainerNode(node *yaml.Node, path string) error {
	if node.Kind == yaml.ScalarNode {
		return validateString(node, path)
	}
	return validateMapping(node, path, yamlMappingSchema{fields: map[string]yamlValidator{
		"image":       validateString,
		"credentials": validateCredentialsNode,
		"env":         validateExtensionScalars,
		"ports":       validateStringOrIntegerSequence,
		"volumes":     validateStringSequence,
		"options":     validateString,
	}})
}

func validateCredentialsNode(node *yaml.Node, path string) error {
	return validateMapping(node, path, yamlMappingSchema{fields: map[string]yamlValidator{
		"username": validateString,
		"password": validateString,
	}})
}

func validateServicesNode(node *yaml.Node, path string) error {
	return validateMapping(node, path, yamlMappingSchema{extension: validateContainerNode})
}

func validateStrategyNode(node *yaml.Node, path string) error {
	return validateMapping(node, path, yamlMappingSchema{fields: map[string]yamlValidator{
		"fail-fast":    validateBooleanOrExpression,
		"max-parallel": validateIntegerOrExpression,
		"matrix":       validateMatrixNode,
	}})
}

func validateMatrixNode(node *yaml.Node, path string) error {
	if node.Kind == yaml.ScalarNode {
		return validateString(node, path)
	}
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("%w: %s: matrix must be a scalar expression or mapping", errStrictYAMLSchema, path)
	}
	return validateMapping(node, path, yamlMappingSchema{extension: validateMatrixValue})
}
