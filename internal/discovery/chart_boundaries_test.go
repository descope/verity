package discovery

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/config"
)

func Test_ExtractChartImages_applies_override_through_public_wrapper(t *testing.T) {
	// Given
	installDiscoveryHelmFake(t)
	chart := config.ChartSpec{Name: "sentinel", Version: "1.0.0", Repository: "https://charts.invalid"}
	overrides := map[string]config.Override{"acme/app": {From: "alpine", To: "wolfi"}}

	// When
	got, err := ExtractChartImages(chart, overrides)

	// Then
	require.NoError(t, err)
	assert.Equal(t, []string{"quay.io/acme/app:1.0-wolfi"}, got)
}

func Test_ExtractChartImagesWithValues_reports_validation_encoding_command_and_manifest_errors(t *testing.T) {
	tests := []struct {
		name        string
		chart       config.ChartSpec
		chartValues map[string]any
		mode        string
		want        string
	}{
		{
			name:  "invalid chart",
			chart: config.ChartSpec{Name: "-bad", Version: "1.0.0", Repository: "https://charts.invalid"},
			want:  "chart name must not start",
		},
		{
			name:        "chart value cannot encode",
			chart:       config.ChartSpec{Name: "sentinel", Version: "1.0.0", Repository: "https://charts.invalid"},
			chartValues: map[string]any{"bad": []any{make(chan int)}},
			want:        "build helm template args",
		},
		{
			name:  "helm command fails",
			chart: config.ChartSpec{Name: "sentinel", Version: "1.0.0", Repository: "https://charts.invalid"},
			mode:  "fail",
			want:  "helm template sentinel",
		},
		{
			name:  "helm output is invalid yaml",
			chart: config.ChartSpec{Name: "sentinel", Version: "1.0.0", Repository: "https://charts.invalid"},
			mode:  "invalid-yaml",
			want:  "extracting images from sentinel manifests",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given
			installDiscoveryHelmFake(t)
			t.Setenv("DISCOVERY_HELM_MODE", tt.mode)

			// When
			_, err := ExtractChartImagesWithValues(tt.chart, nil, tt.chartValues)

			// Then
			require.ErrorContains(t, err, tt.want)
		})
	}
}

func Test_helm_value_helpers_cover_json_nil_and_float32_boundaries(t *testing.T) {
	// When / Then
	_, _, err := helmSetJSONValue("bad", []any{make(chan int)})
	require.ErrorContains(t, err, "failed to JSON-encode")
	_, _, err, handled := tryHelmSetJSON("nil", nil)
	require.NoError(t, err)
	assert.False(t, handled)
	flag, encoded, err := helmSetPair("ratio", float32(1.25))
	require.NoError(t, err)
	assert.Equal(t, helmSetFlag, flag)
	assert.Equal(t, "1.25", encoded)
}

func Test_extractImagesFromManifests_skips_empty_documents(t *testing.T) {
	// When
	got, err := extractImagesFromManifests([]byte("---\n---\n"))

	// Then
	require.NoError(t, err)
	assert.Empty(t, got)
}

func Test_collectEnvImages_and_collectArgImages_ignore_malformed_entries(t *testing.T) {
	// Given
	seen := make(map[string]struct{})
	var got []string
	env := []any{
		"not a map",
		map[string]any{"value": "quay.io/acme/missing-name:1"},
		map[string]any{"name": 42, "value": "quay.io/acme/bad-name:1"},
		map[string]any{"name": "HOME", "value": "quay.io/acme/not-image-env:1"},
		map[string]any{"name": "APP_IMAGE"},
		map[string]any{"name": "APP_IMAGE", "value": 42},
		map[string]any{"name": "APP_IMAGE", "value": "quay.io/acme/env:1"},
	}
	args := []any{42, "--image=quay.io/acme/arg:2"}

	// When
	collectEnvImages(env, seen, &got)
	collectArgImages(args, seen, &got)

	// Then
	assert.Equal(t, []string{"quay.io/acme/env:1", "quay.io/acme/arg:2"}, got)
}

func Test_looksLikeImageRef_rejects_each_invalid_reference_boundary(t *testing.T) {
	tests := map[string]bool{
		"":                              false,
		"quay.io/acme/app:bad tag":      false,
		"https://quay.io/acme/app:1":    false,
		"quay.io/acme/app:1@sha256:bad": false,
		"quay.io/acme/app":              false,
		"quay.io/acme/app:bad!":         false,
		"/acme/app:1":                   false,
		"nginx:1":                       false,
		"quay.io//app:1":                false,
		"quay.io/acme/app:1":            true,
	}

	for value, want := range tests {
		t.Run(value, func(t *testing.T) {
			// When
			got := looksLikeImageRef(value)

			// Then
			assert.Equal(t, want, got)
		})
	}
}

func Test_config_loaders_report_nonregular_and_invalid_yaml_inputs(t *testing.T) {
	// Given
	directory := t.TempDir()
	invalid := filepath.Join(t.TempDir(), "invalid.yaml")
	require.NoError(t, os.WriteFile(invalid, []byte("["), 0o644))

	// When / Then
	_, err := LoadVerityConfig(directory)
	require.ErrorContains(t, err, "reading verity config")
	_, err = LoadVerityConfig(invalid)
	require.ErrorContains(t, err, "parsing verity config")
	_, err = LoadChartsFile(directory)
	require.ErrorContains(t, err, "reading charts file")
	_, err = LoadChartsFile(invalid)
	require.ErrorContains(t, err, "parsing charts file")
}

func Test_isExcluded_matches_exact_discovered_name(t *testing.T) {
	// Given
	image := &DiscoveredImage{Name: "acme/sentinel"}

	// When
	got := isExcluded(image, map[string]struct{}{"acme/sentinel": {}})

	// Then
	assert.True(t, got)
}

func installDiscoveryHelmFake(t *testing.T) {
	t.Helper()
	binDir := t.TempDir()
	script := `#!/bin/sh
set -eu
case "${DISCOVERY_HELM_MODE:-success}" in
fail)
  printf 'sentinel helm failure\n' >&2
  exit 7
  ;;
invalid-yaml)
  printf '[\n'
  ;;
*)
  printf '%s\n' \
    'apiVersion: v1' \
    'kind: Pod' \
    'spec:' \
    '  containers:' \
    '    - image: quay.io/acme/app:1.0-alpine'
  ;;
esac
`
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "helm"), []byte(script), 0o755))
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("DISCOVERY_HELM_MODE", "success")
}
