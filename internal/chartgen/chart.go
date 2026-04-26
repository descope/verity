package chartgen

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/verity-org/verity/internal/config"
	"github.com/verity-org/verity/internal/discovery"
)

// WrapperChart holds the generated wrapper chart YAML content.
type WrapperChart struct {
	Name       string
	Version    string
	ChartYAML  []byte
	ValuesYAML []byte
}

// BuildWrapperChart creates a wrapper Helm chart that depends on the original
// chart and overrides image values with patched references.
func BuildWrapperChart(original config.ChartSpec, overrides []ValueOverride) (*WrapperChart, error) {
	if original.Name == "" {
		return nil, ErrEmptyChartName
	}
	if err := discovery.ValidateChartSpec(original); err != nil {
		return nil, fmt.Errorf("validate chart spec: %w", err)
	}

	wrapperName := original.Name
	wrapperVersion := original.Version

	chartDoc := map[string]any{
		"apiVersion":  "v2",
		"name":        wrapperName,
		"description": fmt.Sprintf("Verity-patched wrapper for %s %s", original.Name, original.Version),
		"type":        "application",
		"version":     wrapperVersion,
		"dependencies": []map[string]string{{
			"name":       original.Name,
			"version":    original.Version,
			"repository": original.Repository,
		}},
		"annotations": map[string]string{
			"verity.supply/source-chart":      original.Name,
			"verity.supply/source-version":    original.Version,
			"verity.supply/source-repository": original.Repository,
		},
	}

	chartYAML, err := yaml.Marshal(chartDoc)
	if err != nil {
		return nil, fmt.Errorf("marshal Chart.yaml: %w", err)
	}

	valuesTree := buildValuesTree(original.Name, overrides)
	valuesYAML, err := yaml.Marshal(valuesTree)
	if err != nil {
		return nil, fmt.Errorf("marshal values.yaml: %w", err)
	}

	return &WrapperChart{
		Name:       wrapperName,
		Version:    wrapperVersion,
		ChartYAML:  chartYAML,
		ValuesYAML: valuesYAML,
	}, nil
}

// PackageChart writes the wrapper chart to a temp dir and runs helm package.
// Returns the path to the .tgz archive. Caller is responsible for cleanup.
func PackageChart(chart *WrapperChart) (string, error) {
	if chart == nil {
		return "", ErrNilChart
	}

	tmpDir, err := os.MkdirTemp("", "verity-wrapper-chart-")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	chartPath := filepath.Join(tmpDir, "Chart.yaml")
	valuesPath := filepath.Join(tmpDir, "values.yaml")

	if err := os.WriteFile(chartPath, chart.ChartYAML, 0o644); err != nil {
		return "", fmt.Errorf("write Chart.yaml: %w", err)
	}
	if err := os.WriteFile(valuesPath, chart.ValuesYAML, 0o644); err != nil {
		return "", fmt.Errorf("write values.yaml: %w", err)
	}
	// Download the declared dependency (the original chart) into charts/.
	// helm package requires dependencies to be present.
	if _, err := runCommand(context.Background(), 5*time.Minute, "helm", "dependency", "build", tmpDir); err != nil {
		return "", fmt.Errorf("helm dependency build: %w", err)
	}

	out, err := runCommand(context.Background(), 5*time.Minute, "helm", "package", tmpDir)
	if err != nil {
		return "", fmt.Errorf("helm package: %w", err)
	}

	const prefix = "Successfully packaged chart and saved it to:"
	for line := range strings.SplitSeq(out, "\n") {
		line = strings.TrimSpace(line)
		if rest, found := strings.CutPrefix(line, prefix); found {
			path := strings.TrimSpace(rest)
			if strings.HasSuffix(path, ".tgz") {
				return path, nil
			}
		}
	}

	return "", ErrNoArchivePath
}

// PushChart pushes a packaged chart archive to an OCI registry.
func PushChart(tgzPath, registry string) error {
	if _, err := runCommand(context.Background(), 5*time.Minute, "helm", "push", tgzPath, registry); err != nil {
		return fmt.Errorf("helm push %s to %s: %w", tgzPath, registry, err)
	}
	return nil
}

// buildValuesTree converts flat dotted-path overrides to a nested map scoped
// under the chart name (Helm dependency override convention).
func buildValuesTree(chartName string, overrides []ValueOverride) map[string]any {
	if len(overrides) == 0 {
		return map[string]any{}
	}

	root := make(map[string]any)
	chartRoot := make(map[string]any)
	root[chartName] = chartRoot

	for _, override := range overrides {
		parts := splitOverridePath(override.Path)
		current := chartRoot

		for i, part := range parts {
			if part == "" {
				continue
			}

			if i == len(parts)-1 {
				if override.Value != "" {
					current[part] = override.Value
					break
				}
				current[part] = map[string]any{
					"repository": override.Repository,
					"tag":        override.Tag,
				}
				break
			}

			next, ok := current[part]
			if !ok {
				nextMap := make(map[string]any)
				current[part] = nextMap
				current = nextMap
				continue
			}

			nextMap, ok := next.(map[string]any)
			if !ok {
				nextMap = make(map[string]any)
				current[part] = nextMap
			}
			current = nextMap
		}
	}

	return root
}

func splitOverridePath(path string) []string {
	if path == "" {
		return nil
	}

	parts := make([]string, 0, strings.Count(path, ".")+1)
	var current strings.Builder
	inBracket := false
	var quote byte

	flush := func() {
		if current.Len() == 0 {
			return
		}
		parts = append(parts, current.String())
		current.Reset()
	}

	for i := range len(path) {
		ch := path[i]
		if inBracket {
			if quote != 0 {
				if ch == quote {
					quote = 0
					continue
				}
				current.WriteByte(ch)
				continue
			}

			switch ch {
			case '\'', '"':
				quote = ch
			case ']':
				inBracket = false
				flush()
			case ' ':
				continue
			default:
				current.WriteByte(ch)
			}
			continue
		}

		switch ch {
		case '.':
			flush()
		case '[':
			flush()
			inBracket = true
		default:
			current.WriteByte(ch)
		}
	}

	flush()
	return parts
}
