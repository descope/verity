package chartgen

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/config"
)

const chartImageKey = "image"

func Test_PackageChart_handles_command_and_output_boundaries(t *testing.T) {
	tests := []struct {
		name      string
		mode      string
		wantError error
		contains  string
	}{
		{name: "success", mode: "success"},
		{name: "dependency command fails", mode: "dependency-fail", contains: "helm dependency build"},
		{name: "package command fails", mode: "package-fail", contains: "helm package"},
		{name: "package output has no archive path", mode: "package-no-path", wantError: ErrNoArchivePath},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given
			fake := installChartgenCommandFakes(t)
			t.Setenv("CHARTGEN_FAKE_HELM_MODE", tt.mode)
			chart := &WrapperChart{
				Name:       "sentinel",
				Version:    "1.0.0",
				ChartYAML:  []byte("apiVersion: v2\nname: sentinel\nversion: 1.0.0\n"),
				ValuesYAML: []byte("replicaCount: 1\n"),
			}

			// When
			got, err := PackageChart(chart)

			// Then
			switch {
			case tt.wantError != nil:
				require.ErrorIs(t, err, tt.wantError)
			case tt.contains != "":
				require.ErrorContains(t, err, tt.contains)
			default:
				require.NoError(t, err)
				assert.Equal(t, fake.packagePath, got)
				assert.FileExists(t, got)
			}
		})
	}
}

func Test_PackageChart_rejects_nil_chart_and_unusable_temp_root(t *testing.T) {
	// Given / When / Then
	_, err := PackageChart(nil)
	require.ErrorIs(t, err, ErrNilChart)

	// Given
	tempRoot := filepath.Join(t.TempDir(), "not-a-directory")
	require.NoError(t, os.WriteFile(tempRoot, []byte("sentinel"), 0o644))
	t.Setenv("TMPDIR", tempRoot)

	// When
	_, err = PackageChart(&WrapperChart{})

	// Then
	require.ErrorContains(t, err, "create temp dir")
}

func Test_BuildWrapperChart_rejects_invalid_nonempty_chart_spec(t *testing.T) {
	// Given
	chart := config.ChartSpec{Name: "sentinel", Version: "1.0.0", Repository: "file:///unsafe"}

	// When
	_, err := BuildWrapperChart(chart, nil, nil)

	// Then
	require.ErrorContains(t, err, "validate chart spec")
}

func Test_splitOverridePath_preserves_bracketed_dots_and_ignores_spacing(t *testing.T) {
	tests := []struct {
		path string
		want []string
	}{
		{path: "", want: nil},
		{path: "global.image.registry", want: []string{"global", chartImageKey, "registry"}},
		{path: `config["a.b"].image`, want: []string{"config", "a.b", chartImageKey}},
		{path: `config['x.y'][ 0 ].tag`, want: []string{"config", "x.y", "0", "tag"}},
		{path: "double..separator", want: []string{"double", "separator"}},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			// When
			got := splitOverridePath(tt.path)

			// Then
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_setScalarValue_replaces_scalar_intermediates_and_skips_empty_segments(t *testing.T) {
	// Given
	root := map[string]any{"service": "legacy"}

	// When
	setScalarValue(root, "service..image.tag", "1.2.3")
	setScalarValue(root, "", "ignored")

	// Then
	assert.Equal(t, map[string]any{
		"service": map[string]any{
			chartImageKey: map[string]any{"tag": "1.2.3"},
		},
	}, root)
}

func Test_mergeMapValue_merges_existing_leaf_and_replaces_scalar_paths(t *testing.T) {
	// Given
	root := map[string]any{
		"existing": map[string]any{
			chartImageKey: map[string]any{"pullPolicy": "Always", "tag": "old"},
		},
		"scalar": "legacy",
	}

	// When
	mergeMapValue(root, "existing.image", map[string]any{"tag": "new"})
	mergeMapValue(root, "scalar.image", map[string]any{"repository": "registry.invalid/sentinel"})
	mergeMapValue(root, "new.image", map[string]any{"tag": "created"})
	mergeMapValue(root, "", map[string]any{"ignored": true})

	// Then
	existing, ok := root["existing"].(map[string]any)
	require.True(t, ok)
	scalar, ok := root["scalar"].(map[string]any)
	require.True(t, ok)
	created, ok := root["new"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, map[string]any{"pullPolicy": "Always", "tag": "new"}, existing[chartImageKey])
	assert.Equal(t, map[string]any{"repository": "registry.invalid/sentinel"}, scalar[chartImageKey])
	assert.Equal(t, map[string]any{"tag": "created"}, created[chartImageKey])
}

func Test_buildChartImageOverrides_handles_nil_missing_and_version_boundaries(t *testing.T) {
	// Given / When / Then
	got, err := buildChartImageOverrides("sentinel", nil, nil)
	require.NoError(t, err)
	assert.Nil(t, got)

	got, err = buildChartImageOverrides("sentinel", nil, &config.VerityConfig{})
	require.NoError(t, err)
	assert.Nil(t, got)

	mapping := ImageMapping{Source: "SOURCE_IMAGE", PatchedRepo: "registry.invalid/sentinel"}
	vc := &config.VerityConfig{ChartImageOverrides: map[string][]config.ChartImageOverride{
		"sentinel": {{Source: "SOURCE_IMAGE", Type: "csv", Path: "images.{version}"}},
	}}

	// When
	_, err = buildChartImageOverrides("sentinel", []ImageMapping{mapping}, vc)

	// Then
	require.ErrorIs(t, err, ErrChartImageOverrideVersion)
	assert.Equal(t, "registry.invalid/sentinel", patchedImageRef(&mapping))
	_, ok := chartImageOverrideVersion(&mapping)
	assert.False(t, ok)
	assert.False(t, errors.Is(err, ErrUnsupportedChartImageOverrideType))
}
