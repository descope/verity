package chartgen

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/config"
)

func Test_Run_generates_wrappers_across_mapping_boundaries(t *testing.T) {
	// Given
	fake := installChartgenCommandFakes(t)
	root := t.TempDir()
	chartsFile := filepath.Join(root, "Chart.yaml")
	verityFile := filepath.Join(root, "verity.yaml")
	writeChartgenFixture(t, chartsFile, `apiVersion: v2
name: sentinel-suite
version: 0.0.0
dependencies:
  - {name: mapped, version: 1.0.0, repository: https://charts.example.invalid}
  - {name: excluded, version: 1.0.0, repository: https://charts.example.invalid}
  - {name: values-only, version: 1.0.0, repository: https://charts.example.invalid}
  - {name: missing, version: 1.0.0, repository: https://charts.example.invalid}
`)
	writeChartgenFixture(t, verityFile, `chartValues:
  values-only:
    feature.enabled: true
unpatchableImages:
  - acme/excluded
`)
	cfg := &Config{
		ChartsFile:     chartsFile,
		VerityConfig:   verityFile,
		TargetRegistry: "registry.example.invalid/patched",
		ChartRegistry:  "oci://registry.example.invalid/charts",
		DryRun:         true,
	}

	// When
	result, err := Run(cfg)

	// Then
	require.NoError(t, err)
	require.Len(t, result.Charts, 3)
	assert.Contains(t, cfg.ExcludeNames, "acme/excluded")
	byName := make(map[string]ChartResult, len(result.Charts))
	for _, chart := range result.Charts {
		byName[chart.Name] = chart
	}
	assert.Len(t, byName["mapped"].ImageMappings, 2)
	assert.Empty(t, byName["excluded"].ImageMappings)
	assert.Empty(t, byName["values-only"].ImageMappings)
	_, missingIncluded := byName["missing"]
	assert.False(t, missingIncluded)
	logData, readErr := os.ReadFile(fake.logPath)
	require.NoError(t, readErr)
	assert.Contains(t, string(logData), "crane:ls registry.example.invalid/patched/acme/mapped")
	assert.NotContains(t, string(logData), "patched/acme/excluded")
}

func Test_Run_packages_pushes_and_removes_archive_when_not_dry_run(t *testing.T) {
	// Given
	fake := installChartgenCommandFakes(t)
	root := t.TempDir()
	chartsFile := filepath.Join(root, "Chart.yaml")
	verityFile := filepath.Join(root, "verity.yaml")
	writeChartgenFixture(t, chartsFile, `apiVersion: v2
name: sentinel-suite
version: 0.0.0
dependencies:
  - {name: mapped, version: 1.0.0, repository: https://charts.example.invalid}
`)
	writeChartgenFixture(t, verityFile, "{}\n")

	// When
	result, err := Run(&Config{
		ChartsFile:     chartsFile,
		VerityConfig:   verityFile,
		TargetRegistry: "registry.example.invalid/patched",
		ChartRegistry:  "oci://registry.example.invalid/charts",
	})

	// Then
	require.NoError(t, err)
	require.Len(t, result.Charts, 1)
	assert.NoFileExists(t, fake.packagePath)
	logData, readErr := os.ReadFile(fake.logPath)
	require.NoError(t, readErr)
	assert.Contains(t, string(logData), "helm:package ")
	assert.Contains(t, string(logData), "helm:push "+fake.packagePath)
}

func Test_processChart_surfaces_or_recovers_from_command_boundary_failures(t *testing.T) {
	tests := []struct {
		name        string
		mode        string
		dryRun      bool
		chartConfig *config.VerityConfig
		wantError   string
	}{
		{name: "template failure", mode: "template-fail", dryRun: true, wantError: "extract images"},
		{name: "values command failure", mode: "values-fail", dryRun: true, wantError: "get chart values"},
		{name: "malformed values", mode: "values-malformed", dryRun: true, wantError: "resolve value paths"},
		{name: "package failure", mode: "package-fail", wantError: "package wrapper chart"},
		{name: "push failure", mode: "push-fail", wantError: "push wrapper chart"},
		{
			name:   "invalid chart image override",
			mode:   "success",
			dryRun: true,
			chartConfig: &config.VerityConfig{ChartImageOverrides: map[string][]config.ChartImageOverride{
				"mapped": {{Source: "", Type: "unsupported", Path: chartImageKey}},
			}},
			wantError: "build chart image overrides",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given
			fake := installChartgenCommandFakes(t)
			t.Setenv("CHARTGEN_FAKE_HELM_MODE", tt.mode)
			vc := tt.chartConfig
			if vc == nil {
				vc = &config.VerityConfig{}
			}

			// When
			_, include, err := processChart(&Config{
				TargetRegistry: "registry.example.invalid/patched",
				ChartRegistry:  "oci://registry.example.invalid/charts",
				DryRun:         tt.dryRun,
			}, mappedChartSpec(), vc)

			// Then
			require.ErrorContains(t, err, tt.wantError)
			assert.False(t, include)
			if tt.mode == "push-fail" {
				assert.NoFileExists(t, fake.packagePath)
			}
		})
	}
}

func Test_processChart_continues_when_subchart_values_cannot_be_loaded(t *testing.T) {
	// Given
	installChartgenCommandFakes(t)
	t.Setenv("CHARTGEN_FAKE_HELM_MODE", "dependency-fail")

	// When
	result, include, err := processChart(&Config{
		TargetRegistry: "registry.example.invalid/patched",
		ChartRegistry:  "oci://registry.example.invalid/charts",
		DryRun:         true,
	}, mappedChartSpec(), &config.VerityConfig{})

	// Then
	require.NoError(t, err)
	assert.True(t, include)
	assert.Equal(t, "mapped", result.Name)
}

func Test_Run_wraps_invalid_input_file_errors(t *testing.T) {
	tests := []struct {
		name       string
		chartsYAML string
		verityYAML string
		want       string
	}{
		{name: "charts file", chartsYAML: "dependencies: [", verityYAML: "{}\n", want: "load charts file"},
		{name: "verity file", chartsYAML: "dependencies: []\n", verityYAML: "chartValues: [", want: "load verity config"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given
			root := t.TempDir()
			chartsFile := filepath.Join(root, "Chart.yaml")
			verityFile := filepath.Join(root, "verity.yaml")
			writeChartgenFixture(t, chartsFile, tt.chartsYAML)
			writeChartgenFixture(t, verityFile, tt.verityYAML)

			// When
			_, err := Run(&Config{ChartsFile: chartsFile, VerityConfig: verityFile})

			// Then
			require.ErrorContains(t, err, tt.want)
		})
	}
}

func Test_sleepContext_observes_timer_and_cancellation_boundaries(t *testing.T) {
	// Given
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	// When / Then
	require.ErrorIs(t, sleepContext(cancelled, time.Hour), context.Canceled)
	require.NoError(t, sleepContext(context.Background(), 0))
}

func mappedChartSpec() config.ChartSpec {
	return config.ChartSpec{Name: "mapped", Version: "1.0.0", Repository: "https://charts.example.invalid"}
}

func writeChartgenFixture(t *testing.T, path, body string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}

func Test_pushChartWithRetry_coerces_nonpositive_attempt_count(t *testing.T) {
	// Given
	var attempts int
	runner := func(context.Context, time.Duration, string, ...string) (string, error) {
		attempts++
		return "", errUnauthorizedPush
	}

	// When
	err := pushChartWithRetry(context.Background(), "sentinel.tgz", "oci://registry.invalid/charts", runner, sleepContext, 0)

	// Then
	require.Error(t, err)
	assert.Equal(t, 1, attempts)
	assert.False(t, isRetriableHelmPushError(nil))
}

func Test_processChart_push_failure_records_only_one_permanent_attempt(t *testing.T) {
	// Given
	fake := installChartgenCommandFakes(t)
	t.Setenv("CHARTGEN_FAKE_HELM_MODE", "push-fail")

	// When
	_, _, err := processChart(&Config{
		TargetRegistry: "registry.example.invalid/patched",
		ChartRegistry:  "oci://registry.example.invalid/charts",
	}, mappedChartSpec(), &config.VerityConfig{})

	// Then
	require.Error(t, err)
	logData, readErr := os.ReadFile(fake.logPath)
	require.NoError(t, readErr)
	assert.Equal(t, 1, strings.Count(string(logData), "helm:push "))
}
