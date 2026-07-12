package ci

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/verity-org/verity/internal/integer/melange"
)

func addPipelineRecipeImpact(impact *integerInputImpact, paths *melange.Paths, lock integerBuildLock) error {
	affected, err := affectedPipelineNames(paths.PipelinesDir, impact.pipelines)
	if err != nil {
		return err
	}
	for key, entry := range lock.Packages {
		uses, err := yamlUses(filepath.Join(paths.LockedDir, filepath.FromSlash(entry.File)))
		if err != nil {
			return fmt.Errorf("read locked recipe %s: %w", key, err)
		}
		if intersects(uses, affected) {
			impact.upstream[key] = struct{}{}
		}
	}
	entries, err := os.ReadDir(paths.BespokeDir)
	if err != nil {
		return fmt.Errorf("list bespoke recipes: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}
		uses, err := yamlUses(filepath.Join(paths.BespokeDir, entry.Name()))
		if err != nil {
			return fmt.Errorf("read bespoke recipe %s: %w", entry.Name(), err)
		}
		if intersects(uses, affected) {
			impact.bespoke[entry.Name()] = struct{}{}
		}
	}
	return nil
}

func affectedPipelineNames(dir string, changed map[string]struct{}) (map[string]struct{}, error) {
	reverse := map[string]map[string]struct{}{}
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".yaml" {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		parent := strings.TrimSuffix(filepath.ToSlash(rel), ".yaml")
		uses, err := yamlUses(path)
		if err != nil {
			return fmt.Errorf("read pipeline %s: %w", parent, err)
		}
		for child := range uses {
			if reverse[child] == nil {
				reverse[child] = map[string]struct{}{}
			}
			reverse[child][parent] = struct{}{}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk bespoke pipelines: %w", err)
	}
	affected := make(map[string]struct{}, len(changed))
	queue := make([]string, 0, len(changed))
	for name := range changed {
		affected[name] = struct{}{}
		queue = append(queue, name)
	}
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		for parent := range reverse[name] {
			if _, seen := affected[parent]; seen {
				continue
			}
			affected[parent] = struct{}{}
			queue = append(queue, parent)
		}
	}
	return affected, nil
}

func yamlUses(path string) (map[string]struct{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	uses := map[string]struct{}{}
	collectYAMLUses(&root, uses)
	return uses, nil
}

func collectYAMLUses(node *yaml.Node, uses map[string]struct{}) {
	if node.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(node.Content); i += 2 {
			key, value := node.Content[i], node.Content[i+1]
			if key.Value == "uses" && value.Kind == yaml.ScalarNode {
				uses[value.Value] = struct{}{}
			}
			collectYAMLUses(value, uses)
		}
		return
	}
	for _, child := range node.Content {
		collectYAMLUses(child, uses)
	}
}

func intersects(left, right map[string]struct{}) bool {
	for value := range left {
		if _, ok := right[value]; ok {
			return true
		}
	}
	return false
}
