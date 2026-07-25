package workflowpolicy

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type workflowFile struct {
	Name     string
	Workflow workflow
}

func ValidateDirectory(directory string) (Report, error) {
	workflows, err := loadWorkflows(directory)
	if err != nil {
		return Report{}, err
	}
	report := Report{WorkflowCount: len(workflows)}
	violations := evaluatePolicies(directory, workflows)
	if len(violations) > 0 {
		return report, &PolicyError{Violations: violations}
	}
	return report, nil
}

func loadWorkflows(directory string) ([]workflowFile, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("%w: read workflow directory %q: %w", ErrInvalidWorkflow, directory, err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	workflows := make([]workflowFile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !isWorkflowFilename(entry.Name()) {
			continue
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("%w: %s: symbolic links are not allowed", ErrInvalidWorkflow, entry.Name())
		}
		path := filepath.Join(directory, entry.Name())
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil, fmt.Errorf("%w: read %q: %w", ErrInvalidWorkflow, path, readErr)
		}
		parsed, parseErr := decodeWorkflow(data)
		if parseErr != nil {
			return nil, fmt.Errorf("%w: %s: %w", ErrInvalidWorkflow, entry.Name(), parseErr)
		}
		if len(parsed.Jobs) == 0 {
			return nil, fmt.Errorf("%w: %s: workflow has no jobs", ErrInvalidWorkflow, entry.Name())
		}
		workflows = append(workflows, workflowFile{Name: entry.Name(), Workflow: parsed})
	}
	if len(workflows) == 0 {
		return nil, fmt.Errorf("%w: no .yaml or .yml workflows in %q", ErrInvalidWorkflow, directory)
	}
	return workflows, nil
}

func decodeWorkflow(data []byte) (workflow, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		return workflow{}, fmt.Errorf("decode YAML: %w", err)
	}
	var trailing yaml.Node
	if err := decoder.Decode(&trailing); err != nil && !errors.Is(err, io.EOF) {
		return workflow{}, fmt.Errorf("decode trailing YAML: %w", err)
	} else if err == nil && len(trailing.Content) > 0 {
		return workflow{}, errMultipleYAMLDocuments
	}
	if err := validateStrictWorkflowYAML(&document); err != nil {
		return workflow{}, fmt.Errorf("validate YAML schema: %w", err)
	}
	var parsed workflow
	if err := document.Content[0].Decode(&parsed); err != nil {
		return workflow{}, fmt.Errorf("decode typed workflow: %w", err)
	}
	return parsed, nil
}

func isWorkflowFilename(name string) bool {
	extension := strings.ToLower(filepath.Ext(name))
	return extension == ".yaml" || extension == ".yml"
}
