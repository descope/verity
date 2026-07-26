package chartgen

import (
	"archive/tar"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/config"
)

func Test_Run_propagates_process_and_strict_mode_errors(t *testing.T) {
	tests := []struct {
		name      string
		chartName string
		mode      string
		strict    bool
		want      string
	}{
		{name: "chart processing", chartName: "mapped", mode: "template-fail", want: "extract images for chart mapped"},
		{name: "strict skipped chart", chartName: "missing", mode: "success", strict: true, want: "strict mode"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given
			installChartgenCommandFakes(t)
			t.Setenv("CHARTGEN_FAKE_HELM_MODE", tt.mode)
			root := t.TempDir()
			chartsFile := filepath.Join(root, "Chart.yaml")
			verityFile := filepath.Join(root, "verity.yaml")
			writeChartgenFixture(t, chartsFile, "dependencies:\n  - name: "+tt.chartName+"\n    version: 1.0.0\n    repository: https://charts.invalid\n")
			writeChartgenFixture(t, verityFile, "{}\n")

			// When
			_, err := Run(&Config{
				ChartsFile: chartsFile, VerityConfig: verityFile,
				TargetRegistry: "registry.invalid/patched", ChartRegistry: "oci://registry.invalid/charts",
				DryRun: true, Strict: tt.strict,
			})

			// Then
			require.ErrorContains(t, err, tt.want)
		})
	}
}

func Test_processChart_tolerates_packaged_archive_cleanup_failure(t *testing.T) {
	// Given
	fake := installChartgenCommandFakes(t)
	t.Setenv("CHARTGEN_FAKE_HELM_MODE", "push-leave-directory")

	// When
	result, include, err := processChart(&Config{
		TargetRegistry: "registry.invalid/patched",
		ChartRegistry:  "oci://registry.invalid/charts",
	}, mappedChartSpec(), &config.VerityConfig{})

	// Then
	require.NoError(t, err)
	assert.True(t, include)
	assert.Equal(t, "mapped", result.Name)
	assert.DirExists(t, fake.packagePath)
}

func Test_chartgen_mapping_helpers_cover_command_sort_and_basename_boundaries(t *testing.T) {
	// Given
	installChartgenCommandFakes(t)
	t.Setenv("CHARTGEN_FAKE_CRANE_MODE", "error")

	// When / Then
	mappings, err := BuildImageMappings([]string{"quay.io/acme/mapped:1.2.3"}, "registry.invalid/patched", nil)
	require.NoError(t, err)
	assert.Empty(t, mappings)

	vc := &config.VerityConfig{Replacements: map[string]config.Replacement{
		"acme/foo": {Registry: "registry.invalid", Image: "foo"},
		"acme/bar": {Registry: "registry.invalid", Image: "bar"},
	}}
	remaining, replacements, excluded := applyReplacements([]string{"quay.io/acme/foo:1", "quay.io/acme/bar:2"}, vc, nil)
	assert.Empty(t, remaining)
	assert.Len(t, replacements, 2)
	assert.Zero(t, excluded)

	assert.Equal(t, "app", nameBasename("quay.io/acme/app:1@sha256:deadbeef"))
	assert.Equal(t, "app", nameBasename("app:1"))
	assert.Equal(t, "app", nameBasename("app"))
}

func Test_log_and_chart_image_override_helpers_cover_empty_and_source_mismatch_paths(t *testing.T) {
	// When / Then
	assert.True(t, logEmptyMappingsAction(config.ChartSpec{Name: "empty", Version: "1.0.0"}, 0, 0, 0, 0))
	got, err := buildChartImageOverrides("sentinel", []ImageMapping{{Source: "other"}}, &config.VerityConfig{
		ChartImageOverrides: map[string][]config.ChartImageOverride{
			"sentinel": {{Source: "wanted", Path: chartImageKey}},
		},
	})
	require.NoError(t, err)
	assert.Empty(t, got)
}

func Test_GetSubchartValues_reports_temp_and_collected_directory_errors(t *testing.T) {
	// Given
	originalTemp := os.Getenv("TMPDIR")
	tempFile := filepath.Join(t.TempDir(), "not-a-directory")
	require.NoError(t, os.WriteFile(tempFile, []byte("sentinel"), 0o644))
	t.Setenv("TMPDIR", tempFile)

	// When
	_, tempErr := GetSubchartValues(mappedChartSpec())

	// Then
	require.ErrorContains(t, tempErr, "create temp dir")

	// Given
	t.Setenv("TMPDIR", originalTemp)
	installChartgenCommandFakes(t)
	t.Setenv("CHARTGEN_FAKE_HELM_MODE", "dependency-charts-file")

	// When
	_, collectErr := GetSubchartValues(mappedChartSpec())

	// Then
	require.ErrorContains(t, collectErr, "read charts directory")
}

func Test_subchart_collectors_propagate_nested_parent_and_missing_directory_boundaries(t *testing.T) {
	// Given
	chartsDir := t.TempDir()
	writeTestChartTarballInDir(t, chartsDir, "parent", map[string]string{
		"Chart.yaml": "name: parent\nversion: 1.0.0\n",
		"charts":     "not a directory",
	})

	// When
	nestedErr := collectNestedSubchartValues(chartsDir, t.TempDir(), map[string][]byte{})
	missingErr := collectSubchartValues(filepath.Join(t.TempDir(), "missing"), map[string][]byte{})

	// Then
	require.ErrorContains(t, nestedErr, "read charts directory")
	require.NoError(t, missingErr)
}

func Test_extractTarball_routes_local_cleaned_traversal_to_safe_path_guard(t *testing.T) {
	// Given
	tgz := writeRawTarball(t, []*tar.Header{{Name: "chart/..", Typeflag: tar.TypeDir, Mode: 0o755}}, []string{""})

	// When
	_, err := extractTarball(tgz, t.TempDir())

	// Then
	require.ErrorIs(t, err, ErrUnsafeTarballEntry)
}
