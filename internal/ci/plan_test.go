package ci

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/integer/apkindex"
)

var errTemporaryAPKIndexOutage = errors.New("temporary apkindex outage")

func TestPlanIntegerPREmptyWhenNoImageFilesChanged(t *testing.T) {
	root := setupIntegerPlanRepo(t)

	plan, err := PlanIntegerPR(&IntegerPROptions{
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

func TestPlanIntegerPRFailsClosedWhenAPKIndexFetchFails(t *testing.T) {
	root := setupIntegerPlanRepo(t)
	originalFetch := apkindexFetch
	apkindexFetch = func(string, string, time.Duration) ([]apkindex.Package, error) {
		return nil, errTemporaryAPKIndexOutage
	}
	t.Cleanup(func() { apkindexFetch = originalFetch })

	_, err := PlanIntegerPR(&IntegerPROptions{
		ChangedFiles: []string{"images/node.yaml"},
		ConfigPath:   filepath.Join(root, "integer.yaml"),
		ImagesDir:    filepath.Join(root, "images"),
		APKIndexURL:  "https://example.invalid/APKINDEX.tar.gz",
		GenDir:       filepath.Join(root, "gen"),
	})
	require.ErrorIs(t, err, errTemporaryAPKIndexOutage)
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

	plan, err := PlanCopaPR(&CopaPROptions{
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

	plan, err := PlanCharts(&ChartOptions{
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

	plan, err := PlanCharts(&ChartOptions{
		EventName:    "pull_request",
		ChangedFiles: []string{"images/grafana.yaml"},
		ChartsFile:   charts,
		ValuesDir:    valuesDir,
	})
	require.NoError(t, err)

	assert.False(t, plan.Strict)
	assert.Equal(t, []map[string]string{{"chart": "grafana"}}, plan.Matrix.Include)
}

func TestPlanChartsIgnoresUnrelatedMakefileChanges(t *testing.T) {
	// Given a repository with a configured chart.
	charts := filepath.Join(t.TempDir(), "Chart.yaml")
	writeTestFile(t, charts, `
dependencies:
  - name: grafana
    version: "10.0.0"
    repository: "https://grafana.github.io/helm-charts"
`)

	// When a pull request changes only a Makefile target unrelated to charts.
	plan, err := PlanCharts(&ChartOptions{
		EventName:    "pull_request",
		ChangedFiles: []string{"Makefile"},
		ChartsFile:   charts,
	})
	require.NoError(t, err)

	// Then chart integration reports its required no-op gate without shards.
	assert.False(t, plan.HasChanges)
	assert.Empty(t, plan.Matrix.Include)
}
