package ci

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/verity-org/verity/internal/integer/apkindex"
	intconfig "github.com/verity-org/verity/internal/integer/config"
)

type integerPackageTestPinning struct {
	recipe    *integerRecipe
	variables map[string]string
	versions  map[string]string
}

type integerYAMLMappingLookup struct {
	node  *yaml.Node
	found bool
}

func pinIntegerPackageTestSpec(data []byte, versions map[string]string) ([]byte, error) {
	var recipe integerRecipe
	if err := yaml.Unmarshal(data, &recipe); err != nil {
		return nil, fmt.Errorf("parse staged package recipe: %w", err)
	}
	variables, err := integerRecipeVariables(&recipe)
	if err != nil {
		return nil, fmt.Errorf("resolve staged package variables: %w", err)
	}
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("parse staged package document: %w", err)
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%w: staged package document must be a mapping", ErrIntegerBatchPlan)
	}
	root := document.Content[0]
	if err := pinIntegerTestNode(root, recipe.Package.Name, versions); err != nil {
		return nil, err
	}
	pinning := integerPackageTestPinning{recipe: &recipe, variables: variables, versions: versions}
	if err := pinning.pinSubpackages(root); err != nil {
		return nil, err
	}
	pinned, err := yaml.Marshal(&document)
	if err != nil {
		return nil, fmt.Errorf("marshal pinned package test spec: %w", err)
	}
	return pinned, nil
}

func (pinning integerPackageTestPinning) pinSubpackages(root *yaml.Node) error {
	lookup, err := integerYAMLMappingValue(root, "subpackages")
	if err != nil || !lookup.found {
		return err
	}
	subpackages := lookup.node
	if subpackages.Kind != yaml.SequenceNode || len(subpackages.Content) != len(pinning.recipe.Subpackages) {
		return fmt.Errorf("%w: invalid staged subpackage sequence", ErrIntegerBatchPlan)
	}
	for index, subpackage := range subpackages.Content {
		name := strings.ReplaceAll(pinning.recipe.Subpackages[index].Name, "${{package.name}}", pinning.recipe.Package.Name)
		name = integerVariablePattern.ReplaceAllStringFunc(name, func(match string) string {
			parts := integerVariablePattern.FindStringSubmatch(match)
			return pinning.variables[parts[1]]
		})
		if strings.Contains(name, "${{") || !integerPackageNamePattern.MatchString(name) {
			return fmt.Errorf("%w: unresolved subpackage name %q", ErrIntegerBatchPlan, name)
		}
		if err := pinIntegerTestNode(subpackage, name, pinning.versions); err != nil {
			return err
		}
	}
	return nil
}

func pinIntegerTestNode(declaration *yaml.Node, name string, versions map[string]string) error {
	lookup, err := integerYAMLMappingValue(declaration, "test")
	if err != nil || !lookup.found {
		return err
	}
	test := lookup.node
	version, exists := versions[name]
	if !exists {
		return fmt.Errorf("%w: tested local package %s missing from index", ErrIntegerBatchPlan, name)
	}
	if !intconfig.ValidMelangeVersion(version) {
		return fmt.Errorf("%w: invalid local package version %q", ErrIntegerBatchPlan, version)
	}
	environment, err := integerEnsureYAMLNode(test, "environment", yaml.MappingNode)
	if err != nil {
		return err
	}
	contents, err := integerEnsureYAMLNode(environment, "contents", yaml.MappingNode)
	if err != nil {
		return err
	}
	packages, err := integerEnsureYAMLNode(contents, "packages", yaml.SequenceNode)
	if err != nil {
		return err
	}
	pin := name + "=" + version
	for _, packageNode := range packages.Content {
		if packageNode.Kind != yaml.ScalarNode {
			return fmt.Errorf("%w: package test dependency must be scalar", ErrIntegerBatchPlan)
		}
		if apkindex.PackageName(packageNode.Value) == name {
			packageNode.Value = pin
			return nil
		}
	}
	packages.Content = append(packages.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: pin})
	return nil
}

func integerEnsureYAMLNode(mapping *yaml.Node, key string, kind yaml.Kind) (*yaml.Node, error) {
	lookup, err := integerYAMLMappingValue(mapping, key)
	if err != nil {
		return nil, err
	}
	value := lookup.node
	if !lookup.found {
		value = &yaml.Node{Kind: kind, Tag: "!!map"}
		if kind == yaml.SequenceNode {
			value.Tag = "!!seq"
		}
		mapping.Content = append(
			mapping.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
			value,
		)
	}
	if value.Kind != kind {
		return nil, fmt.Errorf("%w: %s has invalid YAML shape", ErrIntegerBatchPlan, key)
	}
	return value, nil
}

func integerYAMLMappingValue(mapping *yaml.Node, key string) (integerYAMLMappingLookup, error) {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return integerYAMLMappingLookup{}, fmt.Errorf("%w: expected YAML mapping for %s", ErrIntegerBatchPlan, key)
	}
	for index := 0; index < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value == key {
			return integerYAMLMappingLookup{node: mapping.Content[index+1], found: true}, nil
		}
	}
	return integerYAMLMappingLookup{}, nil
}
