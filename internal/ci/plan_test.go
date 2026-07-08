package ci

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeTestFile(t *testing.T, path, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}

func setupIntegerPlanRepo(t *testing.T) (root string) {
	t.Helper()
	root = t.TempDir()
	writeTestFile(t, filepath.Join(root, "integer.yaml"), `
target:
  registry: ghcr.io/test-org
`)
	for _, base := range []string{"wolfi-base", "wolfi-dev", "wolfi-fips"} {
		writeTestFile(t, filepath.Join(root, "images", "_base", base+".yaml"), "# base\n")
	}
	writeTestFile(t, filepath.Join(root, "images", "node.yaml"), `
name: node
description: Node
upstream:
  package: nodejs-{{version}}
types:
  default:
    base: wolfi-base
    packages: ["nodejs-{{version}}"]
  dev:
    base: wolfi-dev
    packages: ["nodejs-{{version}}", "npm"]
versions:
  "20": {}
  "22": {}
`)
	writeTestFile(t, filepath.Join(root, "images", "curl.yaml"), `
name: curl
description: curl
upstream:
  package: curl
types:
  default:
    base: wolfi-base
    packages: ["curl"]
versions:
  latest:
    latest: true
`)
	return root
}

func TestPlanIntegerPRChangedImageBuildsLatestAndSmokesAllCombos(t *testing.T) {
	root := setupIntegerPlanRepo(t)

	plan, err := PlanIntegerPR(IntegerPROptions{
		ChangedFiles: []string{"images/node.yaml"},
		ConfigPath:   filepath.Join(root, "integer.yaml"),
		ImagesDir:    filepath.Join(root, "images"),
		APKIndexURL:  "",
		GenDir:       filepath.Join(root, "gen"),
	})
	require.NoError(t, err)

	assert.True(t, plan.HasChanges)
	assert.ElementsMatch(t, []map[string]string{
		{"image": "node", "version": "22", "type": "default"},
		{"image": "node", "version": "22", "type": "dev"},
	}, plan.Matrix.Include)
	assert.Len(t, plan.SmokeMatrix.Include, 4)
}

func TestPlanIntegerPREmptyWhenNoImageFilesChanged(t *testing.T) {
	root := setupIntegerPlanRepo(t)

	plan, err := PlanIntegerPR(IntegerPROptions{
		ChangedFiles: []string{"README.md"},
		ConfigPath:   filepath.Join(root, "integer.yaml"),
		ImagesDir:    filepath.Join(root, "images"),
		APKIndexURL:  "",
		GenDir:       filepath.Join(root, "gen"),
	})
	require.NoError(t, err)

	assert.False(t, plan.HasChanges)
	assert.Empty(t, plan.Matrix.Include)
	assert.Empty(t, plan.SmokeMatrix.Include)
}

func TestPlanCopaPRDiffsSemanticImageEntries(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "base-copa.yaml")
	head := filepath.Join(root, "copa.yaml")
	writeTestFile(t, base, `
target:
  registry: ghcr.io/test-org
images:
  - name: library/nginx
    image: mirror.gcr.io/library/nginx
    tags:
      strategy: list
      list: ["1.28.0"]
  - name: library/rabbitmq
    image: mirror.gcr.io/library/rabbitmq
    tags:
      strategy: list
      list: ["4.1.0"]
`)
	writeTestFile(t, head, `
target:
  registry: ghcr.io/test-org
images:
  - name: library/nginx
    image: mirror.gcr.io/library/nginx
    tags:
      strategy: list
      list: ["1.29.0"]
  - name: library/rabbitmq
    image: mirror.gcr.io/library/rabbitmq
    tags:
      strategy: list
      list: ["4.1.0"]
`)

	plan, err := PlanCopaPR(CopaPROptions{
		ChangedFiles:   []string{"copa-config.yaml"},
		BaseConfigPath: base,
		HeadConfigPath: head,
		TargetRegistry: "ghcr.io/test-org",
	})
	require.NoError(t, err)

	assert.True(t, plan.HasChanges)
	assert.Equal(t, []map[string]string{{"name": "library/nginx", "tag": "1.29.0"}}, plan.Matrix.Include)
}

func TestPlanChartsMarksChangedDependencyStrict(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "base-Chart.yaml")
	head := filepath.Join(root, "Chart.yaml")
	writeTestFile(t, base, `
dependencies:
  - name: grafana
    version: "10.0.0"
    repository: "https://grafana.github.io/helm-charts"
  - name: loki
    version: "7.0.0"
    repository: "https://grafana.github.io/helm-charts"
`)
	writeTestFile(t, head, `
dependencies:
  - name: grafana
    version: "10.1.0"
    repository: "https://grafana.github.io/helm-charts"
  - name: loki
    version: "7.0.0"
    repository: "https://grafana.github.io/helm-charts"
`)

	plan, err := PlanCharts(ChartOptions{
		EventName:      "pull_request",
		ChangedFiles:   []string{"Chart.yaml"},
		ChartsFile:     head,
		BaseChartsFile: base,
	})
	require.NoError(t, err)

	assert.True(t, plan.Strict)
	assert.Equal(t, []map[string]string{{"chart": "grafana"}}, plan.Matrix.Include)
}

func TestPlanChartsMapsChangedImageThroughValuesFile(t *testing.T) {
	root := t.TempDir()
	charts := filepath.Join(root, "Chart.yaml")
	valuesDir := filepath.Join(root, "values")
	writeTestFile(t, charts, `
dependencies:
  - name: grafana
    version: "10.0.0"
    repository: "https://grafana.github.io/helm-charts"
  - name: loki
    version: "7.0.0"
    repository: "https://grafana.github.io/helm-charts"
`)
	writeTestFile(t, filepath.Join(valuesDir, "grafana.yaml"), "image: ghcr.io/verity-org/grafana:12\n")

	plan, err := PlanCharts(ChartOptions{
		EventName:    "pull_request",
		ChangedFiles: []string{"images/grafana.yaml"},
		ChartsFile:   charts,
		ValuesDir:    valuesDir,
	})
	require.NoError(t, err)

	assert.False(t, plan.Strict)
	assert.Equal(t, []map[string]string{{"chart": "grafana"}}, plan.Matrix.Include)
}
