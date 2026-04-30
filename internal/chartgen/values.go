package chartgen

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
				clearRegistry := valuePathHasRegistry(values, override.ValuePath)
				result = append(result, ValueOverride{
					Path:          override.ValuePath,
					Repository:    m.PatchedRepo,
					Tag:           m.PatchedTag,
					ClearRegistry: clearRegistry,
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
					Path:          pair.Path,
					Repository:    m.PatchedRepo,
					Tag:           m.PatchedTag,
					ClearRegistry: pair.HasRegistry,
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

// GetSubchartValues fetches the parent chart dependencies and returns each
// subchart values.yaml keyed by subchart name.
func GetSubchartValues(chart config.ChartSpec) (map[string][]byte, error) {
	if err := discovery.ValidateChartSpec(chart); err != nil {
		return nil, fmt.Errorf("validate chart spec: %w", err)
	}

	tmpDir, err := os.MkdirTemp("", "verity-chartgen-subcharts-")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	chartYAML, err := yaml.Marshal(map[string]any{
		"apiVersion": "v2",
		"name":       "verity-subchart-values",
		"version":    "0.0.0",
		"dependencies": []map[string]string{{
			"name":       chart.Name,
			"version":    chart.Version,
			"repository": chart.Repository,
		}},
	})
	if err != nil {
		return nil, fmt.Errorf("marshal temp Chart.yaml: %w", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "Chart.yaml"), chartYAML, 0o644); err != nil {
		return nil, fmt.Errorf("write temp Chart.yaml: %w", err)
	}

	if _, err := runCommand(context.Background(), 5*time.Minute, "helm", "dependency", "build", tmpDir); err != nil {
		return nil, fmt.Errorf("helm dependency build for %s: %w", chart.Name, err)
	}

	chartsDir := filepath.Join(tmpDir, "charts")
	subchartValues := make(map[string][]byte)

	archives, err := enumerateSubchartArchives(chartsDir)
	if err != nil {
		return nil, err
	}
	for _, archive := range archives {
		valuesYAML, chartName, err := extractValuesFromTarball(archive)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: skipping unreadable subchart archive %s: %v\n", filepath.Base(archive), err)
			continue
		}
		if chartName == "" || len(valuesYAML) == 0 {
			continue
		}
		subchartValues[chartName] = valuesYAML
	}

	entries, err := os.ReadDir(chartsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return subchartValues, nil
		}
		return nil, fmt.Errorf("read charts directory: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		valuesPath := filepath.Join(chartsDir, entry.Name(), "values.yaml")
		valuesYAML, err := os.ReadFile(valuesPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			fmt.Fprintf(os.Stderr, "warning: skipping unreadable subchart values %s: %v\n", valuesPath, err)
			continue
		}
		subchartValues[entry.Name()] = valuesYAML
	}

	return subchartValues, nil
}

func enumerateSubchartArchives(chartsDir string) ([]string, error) {
	entries, err := os.ReadDir(chartsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read charts directory: %w", err)
	}

	archives := make([]string, 0)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".tgz" {
			continue
		}
		archives = append(archives, filepath.Join(chartsDir, entry.Name()))
	}
	sort.Strings(archives)
	return archives, nil
}

// extractValuesFromTarball reads a Helm chart tarball and returns the chart
// name together with the raw values.yaml bytes when present.
func extractValuesFromTarball(tgzPath string) (valuesYAML []byte, chartName string, err error) {
	file, err := os.Open(tgzPath)
	if err != nil {
		return nil, "", fmt.Errorf("open tarball %s: %w", filepath.Base(tgzPath), err)
	}
	defer file.Close()

	gzReader, err := gzip.NewReader(file)
	if err != nil {
		return nil, "", fmt.Errorf("open gzip stream for %s: %w", filepath.Base(tgzPath), err)
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			return valuesYAML, chartName, nil
		}
		if err != nil {
			return nil, "", fmt.Errorf("read tarball %s: %w", filepath.Base(tgzPath), err)
		}

		name := strings.TrimPrefix(header.Name, "./")
		if name == "" {
			continue
		}

		parts := strings.Split(name, "/")
		if len(parts) == 0 || parts[0] == "" {
			continue
		}
		if chartName == "" {
			chartName = parts[0]
		}

		if len(parts) == 2 && parts[1] == "values.yaml" {
			valuesYAML, err = io.ReadAll(tarReader)
			if err != nil {
				return nil, "", fmt.Errorf("read values.yaml from %s: %w", filepath.Base(tgzPath), err)
			}
		}
	}
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
			registry, hasRegistry := child["registry"].(string)
			if registry == "" {
				hasRegistry = false
			}

			*pairs = append(*pairs, repoTagPair{
				Path:        path,
				Repo:        repo,
				HasTag:      hasTag,
				Registry:    registry,
				HasRegistry: hasRegistry,
			})
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

func valuePathHasRegistry(values map[string]any, path string) bool {
	parts := splitOverridePath(path)
	if len(parts) == 0 {
		return false
	}

	var current any = values
	for _, part := range parts {
		node, ok := current.(map[string]any)
		if !ok {
			return false
		}

		current, ok = node[part]
		if !ok {
			return false
		}
	}

	node, ok := current.(map[string]any)
	if !ok {
		return false
	}

	registry, ok := node["registry"].(string)
	return ok && registry != ""
}
