package ci

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

const planChartsFixture = `
dependencies:
  - name: beta
    version: "2.0.0"
    repository: "https://example.invalid/charts"
  - name: alpha
    version: "1.0.0"
    repository: "https://example.invalid/charts"
`

func TestPlanCopaPR_handles_noop_empty_discovery_and_config_errors(t *testing.T) {
	t.Run("unrelated change", func(t *testing.T) {
		// Given
		options := CopaPROptions{ChangedFiles: []string{"README.md"}}

		// When
		plan, err := PlanCopaPR(&options)

		// Then
		require.NoError(t, err)
		require.False(t, plan.HasChanges)
		require.Empty(t, plan.Matrix.Include)
	})

	t.Run("semantic no-op", func(t *testing.T) {
		// Given
		root := t.TempDir()
		base := filepath.Join(root, "base.yaml")
		head := filepath.Join(root, "head.yaml")
		body := `
target:
  registry: ghcr.io/sentinel
images:
  - name: library/demo
    image: mirror.invalid/library/demo
    tags:
      strategy: list
      list: ["1.0.0"]
`
		writeTestFile(t, base, body)
		writeTestFile(t, head, body)

		// When
		plan, err := PlanCopaPR(&CopaPROptions{ChangedFiles: []string{"copa-config.yaml"}, BaseConfigPath: base, HeadConfigPath: head})

		// Then
		require.NoError(t, err)
		require.False(t, plan.HasChanges)
		require.Empty(t, plan.Matrix.Include)
	})

	t.Run("changed image with no selected tags", func(t *testing.T) {
		// Given
		root := t.TempDir()
		base := filepath.Join(root, "base.yaml")
		head := filepath.Join(root, "head.yaml")
		writeTestFile(t, base, `target: {registry: ghcr.io/sentinel}
images:
  - name: library/demo
    image: mirror.invalid/library/demo
    tags: {strategy: list, list: ["1.0.0"]}
`)
		writeTestFile(t, head, `target: {registry: ghcr.io/sentinel}
images:
  - name: library/demo
    image: mirror.invalid/library/demo
    tags: {strategy: list, list: []}
`)

		// When
		plan, err := PlanCopaPR(&CopaPROptions{ChangedFiles: []string{"copa-config.yaml"}, BaseConfigPath: base, HeadConfigPath: head})

		// Then
		require.NoError(t, err)
		require.False(t, plan.HasChanges)
	})

	t.Run("missing head", func(t *testing.T) {
		// Given
		options := CopaPROptions{ChangedFiles: []string{"copa-config.yaml"}, HeadConfigPath: filepath.Join(t.TempDir(), "missing.yaml")}

		// When
		_, err := PlanCopaPR(&options)

		// Then
		require.ErrorContains(t, err, "load head copa config")
	})

	t.Run("missing base", func(t *testing.T) {
		// Given
		root := t.TempDir()
		head := filepath.Join(root, "head.yaml")
		writeTestFile(t, head, `target: {registry: ghcr.io/sentinel}
images: []
`)

		// When
		_, err := PlanCopaPR(&CopaPROptions{ChangedFiles: []string{"copa-config.yaml"}, BaseConfigPath: filepath.Join(root, "missing.yaml"), HeadConfigPath: head})

		// Then
		require.ErrorContains(t, err, "load base copa config")
	})

	t.Run("failed image discovery becomes an empty plan", func(t *testing.T) {
		// Given
		root := t.TempDir()
		head := filepath.Join(root, "head.yaml")
		writeTestFile(t, head, `target: {registry: ghcr.io/sentinel}
images:
  - name: library/demo
    image: mirror.invalid/library/demo
    tags: {strategy: unsupported}
`)

		// When
		plan, err := PlanCopaPR(&CopaPROptions{ChangedFiles: []string{"copa-config.yaml"}, HeadConfigPath: head})

		// Then
		require.NoError(t, err)
		require.False(t, plan.HasChanges)
		require.Empty(t, plan.Matrix.Include)
	})
}

func TestPlanCharts_selects_explicit_scheduled_and_broad_changes(t *testing.T) {
	// Given
	charts := filepath.Join(t.TempDir(), "Chart.yaml")
	writeTestFile(t, charts, planChartsFixture)
	tests := []struct {
		name    string
		options ChartOptions
		want    []map[string]string
	}{
		{name: "explicit chart", options: ChartOptions{InputChart: " alpha ", ChartsFile: charts}, want: []map[string]string{{"chart": "alpha"}}},
		{name: "scheduled build", options: ChartOptions{EventName: "schedule", ChartsFile: charts}, want: []map[string]string{{"chart": "alpha"}, {"chart": "beta"}}},
		{name: "broad workflow change", options: ChartOptions{EventName: "pull_request", ChangedFiles: []string{".github/workflows/chart-integration.yaml"}, ChartsFile: charts}, want: []map[string]string{{"chart": "alpha"}, {"chart": "beta"}}},
		{name: "base image change", options: ChartOptions{EventName: "pull_request", ChangedFiles: []string{"images/_base/common.yaml"}, ChartsFile: charts}, want: []map[string]string{{"chart": "alpha"}, {"chart": "beta"}}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// When
			plan, err := PlanCharts(&test.options)

			// Then
			require.NoError(t, err)
			require.True(t, plan.HasChanges)
			require.Equal(t, test.want, plan.Matrix.Include)
		})
	}
}

func TestPlanCharts_reports_chart_dependency_and_image_config_errors(t *testing.T) {
	t.Run("missing optional chart file", func(t *testing.T) {
		// Given
		options := ChartOptions{ChartsFile: filepath.Join(t.TempDir(), "missing.yaml")}

		// When
		plan, err := PlanCharts(&options)

		// Then
		require.NoError(t, err)
		require.False(t, plan.HasChanges)
		require.Empty(t, plan.Matrix.Include)
	})

	t.Run("malformed chart file", func(t *testing.T) {
		// Given
		path := filepath.Join(t.TempDir(), "Chart.yaml")
		writeTestFile(t, path, "dependencies: [")

		// When
		_, err := PlanCharts(&ChartOptions{ChartsFile: path})

		// Then
		require.ErrorContains(t, err, "load charts")
	})

	t.Run("missing base dependency file", func(t *testing.T) {
		// Given
		root := t.TempDir()
		charts := filepath.Join(root, "Chart.yaml")
		writeTestFile(t, charts, planChartsFixture)

		// When
		_, err := PlanCharts(&ChartOptions{EventName: "pull_request", ChangedFiles: []string{"Chart.yaml"}, ChartsFile: charts, BaseChartsFile: filepath.Join(root, "missing.yaml")})

		// Then
		require.ErrorContains(t, err, "load base Chart.yaml")
	})

	t.Run("malformed Verity config", func(t *testing.T) {
		// Given
		root := t.TempDir()
		charts := filepath.Join(root, "Chart.yaml")
		verity := filepath.Join(root, "verity.yaml")
		writeTestFile(t, charts, planChartsFixture)
		writeTestFile(t, verity, "chartValues: [")

		// When
		_, err := PlanCharts(&ChartOptions{EventName: "pull_request", ChangedFiles: []string{"images/alpha.yaml"}, ChartsFile: charts, VerityConfig: verity})

		// Then
		require.ErrorContains(t, err, "load verity config")
	})

	t.Run("malformed dependency file", func(t *testing.T) {
		// Given
		path := filepath.Join(t.TempDir(), "Chart.yaml")
		writeTestFile(t, path, "dependencies: [")

		// When
		_, err := loadChartDepMap(path)

		// Then
		require.Error(t, err)
	})

	t.Run("missing head dependency file", func(t *testing.T) {
		// Given
		missing := filepath.Join(t.TempDir(), "missing.yaml")

		// When
		_, err := changedChartDependencies("", missing)

		// Then
		require.ErrorContains(t, err, "load head Chart.yaml")
	})
}
