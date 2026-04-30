package chartgen

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/verity-org/verity/internal/config"
	"github.com/verity-org/verity/internal/discovery"
)

type ValueOverride struct {
	Path          string `json:"path"`
	Repository    string `json:"repository"`
	Tag           string `json:"tag"`
	Value         string `json:"value,omitempty"`
	ClearRegistry bool   `json:"clearRegistry,omitempty"`
}

type repoTagPair struct {
	Path        string
	Repo        string
	HasTag      bool
	Registry    string
	HasRegistry bool
}

func ResolveValuePaths(valuesYAML []byte, mappings []ImageMapping, overrides map[string]config.Override) ([]ValueOverride, error) {
	values := make(map[string]any)
	if err := yaml.Unmarshal(valuesYAML, &values); err != nil {
		return nil, fmt.Errorf("unmarshal values YAML: %w", err)
	}

	result := make([]ValueOverride, 0, len(mappings))
	matched := make([]bool, len(mappings))

	overrideKeys := make([]string, 0, len(overrides))
	for key := range overrides {
		overrideKeys = append(overrideKeys, key)
	}
	sort.Strings(overrideKeys)

	for i, m := range mappings {
		for _, key := range overrideKeys {
			override := overrides[key]
			if override.ValuePath == "" {
				continue
			}
			if matchesRepo(m.OriginalRepo, key) {
				result = append(result, ValueOverride{
					Path:       override.ValuePath,
					Repository: m.PatchedRepo,
					Tag:        m.PatchedTag,
				})
				matched[i] = true
				break
			}
		}
	}

	var pairs []repoTagPair
	walkValues("", values, &pairs)
	sort.Slice(pairs, func(i, j int) bool {
		return pairs[i].Path < pairs[j].Path
	})

	for _, pair := range pairs {
		if !pair.HasTag {
			continue
		}
		for i, m := range mappings {
			if matched[i] {
				continue
			}
			if matchesRepo(pair.Repo, m.OriginalRepo) {
				result = append(result, ValueOverride{
					Path:       pair.Path,
					Repository: m.PatchedRepo,
					Tag:        m.PatchedTag,
				})
				matched[i] = true
				break
			}
		}
	}

	return result, nil
}

func GetChartValues(chart config.ChartSpec) ([]byte, error) {
	if err := discovery.ValidateChartSpec(chart); err != nil {
		return nil, fmt.Errorf("validate chart spec: %w", err)
	}

	args := []string{"show", "values"}
	if strings.HasPrefix(chart.Repository, "oci://") {
		args = append(args, chart.Repository+"/"+chart.Name)
	} else {
		args = append(args, chart.Name, "--repo", chart.Repository)
	}
	args = append(args, "--version", chart.Version)

	out, err := runCommand(context.Background(), 5*time.Minute, "helm", args...)
	if err != nil {
		return nil, fmt.Errorf("get chart values for %s: %w", chart.Name, err)
	}

	return []byte(out), nil
}

func walkValues(prefix string, node map[string]any, pairs *[]repoTagPair) {
	for key, val := range node {
		child, ok := val.(map[string]any)
		if !ok {
			continue
		}

		path := key
		if prefix != "" {
			path = prefix + "." + key
		}

		repo, hasRepo := child["repository"].(string)
		if hasRepo && repo != "" {
			_, hasTag := child["tag"]
			*pairs = append(*pairs, repoTagPair{Path: path, Repo: repo, HasTag: hasTag})
		}

		walkValues(path, child, pairs)
	}
}

func matchesRepo(imageRepo, candidate string) bool {
	if imageRepo == candidate {
		return true
	}
	if strings.HasSuffix(imageRepo, "/"+candidate) {
		return true
	}
	if strings.HasSuffix(candidate, "/"+imageRepo) {
		return true
	}
	return false
}
