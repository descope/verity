package workflowpolicy

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

type workflow struct {
	Name        string                 `yaml:"name"`
	On          triggers               `yaml:"on"`
	Permissions permissions            `yaml:"permissions"`
	Env         scalarMap              `yaml:"env"`
	Jobs        map[string]workflowJob `yaml:"jobs"`
}

type workflowJob struct {
	Permissions permissions              `yaml:"permissions"`
	Needs       stringList               `yaml:"needs"`
	If          string                   `yaml:"if"`
	Uses        string                   `yaml:"uses"`
	With        scalarMap                `yaml:"with"`
	Outputs     scalarMap                `yaml:"outputs"`
	Env         scalarMap                `yaml:"env"`
	Secrets     reusableJobSecrets       `yaml:"secrets"`
	Container   containerSpec            `yaml:"container"`
	Services    map[string]containerSpec `yaml:"services"`
	Strategy    workflowStrategy         `yaml:"strategy"`
	Steps       []workflowStep           `yaml:"steps"`
}

type workflowStrategy struct {
	Matrix yaml.Node `yaml:"matrix"`
}

type workflowStep struct {
	Name             string      `yaml:"name"`
	ID               string      `yaml:"id"`
	If               string      `yaml:"if"`
	Uses             string      `yaml:"uses"`
	Run              string      `yaml:"run"`
	Shell            string      `yaml:"shell"`
	WorkingDirectory string      `yaml:"working-directory"`
	With             scalarMap   `yaml:"with"`
	Env              scalarMap   `yaml:"env"`
	ContinueOnError  scalarValue `yaml:"continue-on-error"`
}

type scalarValue struct {
	set   bool
	value string
}

func (v *scalarValue) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.ScalarNode {
		return fmt.Errorf("%w: got kind %d", errExpectedScalar, node.Kind)
	}
	v.set = true
	v.value = node.Value
	return nil
}

type scalarMap map[string]string

func (m *scalarMap) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("%w: got kind %d", errExpectedMapping, node.Kind)
	}
	values := make(scalarMap, len(node.Content)/2)
	for index := 0; index < len(node.Content); index += 2 {
		key := node.Content[index]
		value := node.Content[index+1]
		if key.Kind != yaml.ScalarNode || value.Kind != yaml.ScalarNode {
			return errMappingScalars
		}
		values[key.Value] = value.Value
	}
	*m = values
	return nil
}

type stringList []string

func (values *stringList) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		*values = stringList{node.Value}
		return nil
	case yaml.SequenceNode:
		result := make(stringList, 0, len(node.Content))
		for _, child := range node.Content {
			if child.Kind != yaml.ScalarNode {
				return errTriggerListScalars
			}
			result = append(result, child.Value)
		}
		*values = result
		return nil
	default:
		return fmt.Errorf("needs: %w", errTriggerShape)
	}
}

type reusableJobSecrets struct {
	set     bool
	inherit bool
	values  map[string]string
}

func (secrets *reusableJobSecrets) UnmarshalYAML(node *yaml.Node) error {
	secrets.set = true
	if node.Kind == yaml.ScalarNode {
		secrets.inherit = node.Value == "inherit"
		return nil
	}
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("secrets: %w", errExpectedMapping)
	}
	secrets.values = make(map[string]string, len(node.Content)/2)
	for index := 0; index < len(node.Content); index += 2 {
		secrets.values[node.Content[index].Value] = node.Content[index+1].Value
	}
	return nil
}

type containerSpec struct {
	Image   string     `yaml:"image"`
	Env     scalarMap  `yaml:"env"`
	Volumes stringList `yaml:"volumes"`
	Options string     `yaml:"options"`
}

func (c *containerSpec) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		c.Image = node.Value
		return nil
	case yaml.MappingNode:
		type rawContainer containerSpec
		var raw rawContainer
		if err := node.Decode(&raw); err != nil {
			return fmt.Errorf("decode container: %w", err)
		}
		*c = containerSpec(raw)
		return nil
	default:
		return errContainerShape
	}
}
