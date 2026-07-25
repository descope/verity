package workflowpolicy

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type runLocation struct {
	file             string
	job              string
	stepID           string
	workingDirectory string
	isWorkflow       bool
}

type actionRuns struct {
	Runs struct {
		Steps []workflowStep `yaml:"steps"`
	} `yaml:"runs"`
}

func validateLocalVerityBuildPolicy(directory string, workflows []workflowFile) []Violation {
	violations := validateWorkflowLocalVerityBuilds(workflows)
	cleaned := filepath.Clean(directory)
	githubDirectory := filepath.Dir(cleaned)
	if filepath.Base(cleaned) != "workflows" || filepath.Base(githubDirectory) != ".github" {
		return violations
	}
	root := filepath.Dir(githubDirectory)
	actionViolations, err := validateActionLocalVerityBuilds(root)
	if err != nil {
		return append(violations, buildOnceViolation(".github/actions", "", err.Error()))
	}
	return append(violations, actionViolations...)
}

func validateWorkflowLocalVerityBuilds(workflows []workflowFile) []Violation {
	var violations []Violation
	for workflowIndex := range workflows {
		file := &workflows[workflowIndex]
		for _, jobName := range sortedJobNames(file.Workflow.Jobs) {
			job := file.Workflow.Jobs[jobName]
			for stepIndex := range job.Steps {
				step := &job.Steps[stepIndex]
				location := runLocation{
					file:             file.Name,
					job:              jobName,
					stepID:           step.ID,
					workingDirectory: step.WorkingDirectory,
					isWorkflow:       true,
				}
				violations = append(violations, localVerityBuildViolations(location, step.Run)...)
			}
		}
	}
	return violations
}

func validateActionLocalVerityBuilds(root string) ([]Violation, error) {
	actionsRootPath := filepath.Join(root, ".github", "actions")
	actionsRoot, err := os.OpenRoot(actionsRootPath)
	if err != nil {
		return nil, fmt.Errorf("open composite actions root %q: %w", actionsRootPath, err)
	}
	var violations []Violation
	walkErr := fs.WalkDir(actionsRoot.FS(), ".", func(actionPath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || (entry.Name() != "action.yml" && entry.Name() != "action.yaml") {
			return nil
		}
		data, err := actionsRoot.ReadFile(actionPath)
		if err != nil {
			return fmt.Errorf("read composite action %q: %w", actionPath, err)
		}
		var action actionRuns
		if err := yaml.Unmarshal(data, &action); err != nil {
			return fmt.Errorf("decode composite action %q: %w", actionPath, err)
		}
		relative := filepath.Join(".github", "actions", filepath.FromSlash(actionPath))
		for stepIndex := range action.Runs.Steps {
			step := &action.Runs.Steps[stepIndex]
			location := runLocation{
				file:             filepath.ToSlash(relative),
				job:              "composite",
				stepID:           step.ID,
				workingDirectory: step.WorkingDirectory,
			}
			violations = append(violations, localVerityBuildViolations(location, step.Run)...)
		}
		return nil
	})
	closeErr := actionsRoot.Close()
	if walkErr != nil {
		return nil, fmt.Errorf("scan composite actions: %w", walkErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close composite actions root: %w", closeErr)
	}
	return violations, nil
}

func localVerityBuildViolations(location runLocation, script string) []Violation {
	for _, command := range splitShellCommands(script) {
		if reason := localVerityCompilationReason(location, command); reason != "" {
			return []Violation{buildOnceViolation(
				location.file,
				location.job,
				"local Verity compilation is forbidden outside the controlled build-verity helper: "+reason,
			)}
		}
	}
	return nil
}

func cleanShellPath(value string) string {
	return filepath.ToSlash(filepath.Clean(strings.TrimSpace(value)))
}
