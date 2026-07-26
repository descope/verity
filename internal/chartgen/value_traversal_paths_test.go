package chartgen

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/config"
)

func Test_collectNestedSubchartValues_traverses_archived_and_directory_parents(t *testing.T) {
	// Given
	chartsDir := t.TempDir()
	extractDir := t.TempDir()
	writeTestChartTarballInDir(t, chartsDir, "archived-parent", map[string]string{
		"Chart.yaml":                  "name: archived-parent\nversion: 1.0.0\n",
		"charts/archived/values.yaml": "image: archived\n",
		"charts/no-values/Chart.yaml": "name: no-values\nversion: 1.0.0\n",
		"charts/archived/extra.txt":   "sentinel\n",
		"charts/no-values/extra.txt":  "sentinel\n",
		"charts/archived/Chart.yaml":  "name: archived\nversion: 1.0.0\n",
		"charts/no-values/README.md":  "sentinel\n",
		"charts/archived/templates/x": "sentinel\n",
	})
	require.NoError(t, os.WriteFile(filepath.Join(chartsDir, "broken.tgz"), []byte("not gzip"), 0o644))
	mustWriteFile(t, filepath.Join(chartsDir, "direct-parent", "charts", "direct", "values.yaml"), "image: direct\n")
	require.NoError(t, os.WriteFile(filepath.Join(chartsDir, "notes.txt"), []byte("ignored"), 0o644))
	got := make(map[string][]byte)

	// When
	err := collectNestedSubchartValues(chartsDir, extractDir, got)

	// Then
	require.NoError(t, err)
	assert.Equal(t, "image: archived\n", string(got["archived"]))
	assert.Equal(t, "image: direct\n", string(got["direct"]))
	assert.NotContains(t, got, "no-values")
}

func Test_collectSubchartValues_skips_unreadable_and_empty_sources(t *testing.T) {
	// Given
	chartsDir := t.TempDir()
	writeTestChartTarballInDir(t, chartsDir, "archive-child", map[string]string{
		"Chart.yaml":  "name: archive-child\nversion: 1.2.3\n",
		"values.yaml": "image: archive\n",
	})
	writeTestChartTarballInDir(t, chartsDir, "empty-child", map[string]string{
		"Chart.yaml": "name: empty-child\nversion: 1.0.0\n",
	})
	require.NoError(t, os.WriteFile(filepath.Join(chartsDir, "broken.tgz"), []byte("not gzip"), 0o644))
	mustWriteFile(t, filepath.Join(chartsDir, "directory-child", "values.yaml"), "image: directory\n")
	require.NoError(t, os.MkdirAll(filepath.Join(chartsDir, "missing-values"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(chartsDir, "unreadable-values", "values.yaml"), 0o755))
	got := make(map[string][]byte)

	// When
	err := collectSubchartValues(chartsDir, got)

	// Then
	require.NoError(t, err)
	assert.Equal(t, "image: archive\n", string(got["archive-child"]))
	assert.Equal(t, "image: directory\n", string(got["directory-child"]))
	assert.NotContains(t, got, "empty-child")
	assert.NotContains(t, got, "missing-values")
	assert.NotContains(t, got, "unreadable-values")
}

func Test_subchart_traversal_returns_directory_read_errors(t *testing.T) {
	// Given
	path := filepath.Join(t.TempDir(), "charts-file")
	require.NoError(t, os.WriteFile(path, []byte("sentinel"), 0o644))

	// When
	_, enumerateErr := enumerateSubchartArchives(path)
	nestedErr := collectNestedSubchartValues(path, t.TempDir(), map[string][]byte{})
	collectErr := collectSubchartValues(path, map[string][]byte{})

	// Then
	require.ErrorContains(t, enumerateErr, "read charts directory")
	require.ErrorContains(t, nestedErr, "read charts directory")
	require.ErrorContains(t, collectErr, "read charts directory")
}

func Test_subchart_traversal_treats_missing_or_empty_parent_as_empty(t *testing.T) {
	// Given
	missing := filepath.Join(t.TempDir(), "missing")

	// When
	archives, enumerateErr := enumerateSubchartArchives(missing)
	parentErr := collectParentSubcharts("", map[string][]byte{})
	nestedErr := collectNestedSubchartValues(missing, t.TempDir(), map[string][]byte{})

	// Then
	require.NoError(t, enumerateErr)
	assert.Empty(t, archives)
	require.NoError(t, parentErr)
	require.NoError(t, nestedErr)
}

func Test_writeSubchartDependencyChart_writes_dependency_contract(t *testing.T) {
	// Given
	root := t.TempDir()
	chart := config.ChartSpec{Name: "sentinel", Version: "1.2.3", Repository: "oci://registry.invalid/charts"}

	// When
	err := writeSubchartDependencyChart(root, chart)

	// Then
	require.NoError(t, err)
	data, readErr := os.ReadFile(filepath.Join(root, "Chart.yaml"))
	require.NoError(t, readErr)
	assert.Contains(t, string(data), "name: sentinel")
	assert.Contains(t, string(data), "repository: oci://registry.invalid/charts")
}

func Test_writeSubchartDependencyChart_returns_write_error_for_non_directory(t *testing.T) {
	// Given
	path := filepath.Join(t.TempDir(), "not-a-directory")
	require.NoError(t, os.WriteFile(path, []byte("sentinel"), 0o644))

	// When
	err := writeSubchartDependencyChart(path, mappedChartSpec())

	// Then
	require.ErrorContains(t, err, "write temp Chart.yaml")
}

func Test_GetChartValues_builds_oci_reference_and_returns_command_output(t *testing.T) {
	// Given
	fake := installChartgenCommandFakes(t)
	chart := config.ChartSpec{Name: "mapped", Version: "1.0.0", Repository: "oci://registry.invalid/charts"}

	// When
	got, err := GetChartValues(chart)

	// Then
	require.NoError(t, err)
	assert.Contains(t, string(got), "quay.io/acme/mapped")
	logData, readErr := os.ReadFile(fake.logPath)
	require.NoError(t, readErr)
	assert.Contains(t, string(logData), "helm:show values oci://registry.invalid/charts/mapped --version 1.0.0")
}

func Test_GetChartValues_wraps_validation_and_command_errors(t *testing.T) {
	// Given
	installChartgenCommandFakes(t)
	invalid := config.ChartSpec{Name: "mapped", Version: "1.0.0", Repository: "file:///invalid"}

	// When
	_, validationErr := GetChartValues(invalid)
	t.Setenv("CHARTGEN_FAKE_HELM_MODE", "values-fail")
	_, commandErr := GetChartValues(mappedChartSpec())

	// Then
	require.ErrorContains(t, validationErr, "validate chart spec")
	require.ErrorContains(t, commandErr, "get chart values for mapped")
}

func Test_GetSubchartValues_wraps_validation_and_dependency_errors(t *testing.T) {
	// Given
	installChartgenCommandFakes(t)
	invalid := config.ChartSpec{Name: "mapped", Version: "1.0.0", Repository: "file:///invalid"}

	// When
	_, validationErr := GetSubchartValues(invalid)
	t.Setenv("CHARTGEN_FAKE_HELM_MODE", "dependency-fail")
	_, commandErr := GetSubchartValues(mappedChartSpec())

	// Then
	require.ErrorContains(t, validationErr, "validate chart spec")
	require.ErrorContains(t, commandErr, "helm dependency build for mapped")
}
