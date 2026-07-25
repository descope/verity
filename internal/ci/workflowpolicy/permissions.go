package workflowpolicy

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

type permissionLevel string

const (
	permissionNone  permissionLevel = "none"
	permissionRead  permissionLevel = "read"
	permissionWrite permissionLevel = "write"
)

type permissionScope string

type permissions struct {
	declared bool
	all      permissionLevel
	scopes   map[permissionScope]permissionLevel
}

func (p *permissions) UnmarshalYAML(node *yaml.Node) error {
	p.declared = true
	p.scopes = make(map[permissionScope]permissionLevel)
	if node.Kind == yaml.ScalarNode {
		switch node.Value {
		case "read-all":
			p.all = permissionRead
			return nil
		case "write-all":
			p.all = permissionWrite
			return nil
		default:
			return fmt.Errorf("%w %q", errPermissionValue, node.Value)
		}
	}
	if node.Kind != yaml.MappingNode {
		return errPermissionShape
	}
	for index := 0; index < len(node.Content); index += 2 {
		key := permissionScope(node.Content[index].Value)
		value := permissionLevel(node.Content[index+1].Value)
		switch value {
		case permissionRead, permissionWrite, permissionNone:
			p.scopes[key] = value
		default:
			return fmt.Errorf("%w: %s=%q", errPermissionLevel, key, value)
		}
	}
	return nil
}

func (p permissions) level(scope permissionScope) permissionLevel {
	if p.all != "" {
		return p.all
	}
	if level, ok := p.scopes[scope]; ok {
		return level
	}
	return permissionNone
}
