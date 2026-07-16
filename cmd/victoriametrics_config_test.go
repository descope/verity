package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/integer/apkindex"
	intconfig "github.com/verity-org/verity/internal/integer/config"
)

func Test_VictoriaMetrics_uses_fixed_bespoke_package_for_default_variant(t *testing.T) {
	// Given: the checked-in VictoriaMetrics image definition.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "victoriametrics.yaml"))
	require.NoError(t, err)

	// When: the only supported image variant is resolved.
	tmpl := def.Types["default"]

	// Then: it selects only the local rebuild and preserves the runtime contract.
	require.Len(t, def.Types, 1)
	require.NotNil(t, tmpl.Melange)
	require.Equal(t, "victoriametrics.yaml", tmpl.Melange.Bespoke.First())
	require.Equal(t, []string{"victoriametrics"}, tmpl.Packages)
	require.Equal(t, "/usr/bin/victoria-metrics", tmpl.Entrypoint)
	require.Contains(t, tmpl.Paths, intconfig.PathDef{Path: "/victoria-metrics-data", Type: "directory", UID: 65532, GID: 65532, Permissions: "0o755"})
}

func Test_VictoriaMetrics_recipe_pins_immutable_fixed_source_and_runtime_contract(t *testing.T) {
	// Given: the bespoke package recipe selected by the image.
	recipe, err := os.ReadFile(filepath.Join("..", "packages", "bespoke", "victoriametrics.yaml"))
	require.NoError(t, err)
	text := string(recipe)

	// When: its source, toolchain, smoke, and license metadata are inspected.

	// Then: revision r2 owns immutable Apache-2.0 v1.147.0 built with fixed Go.
	require.Contains(t, text, "name: victoriametrics")
	require.Contains(t, text, "# renovate: datasource=github-tags depName=VictoriaMetrics/VictoriaMetrics versioning=semver-coerced")
	require.Contains(t, text, "version: \"1.147.0\"")
	require.Contains(t, text, "epoch: 2")
	require.Contains(t, text, "license: Apache-2.0")
	require.Contains(t, text, "Adapted from wolfi-dev/os@448782c863407612dc8e71620583dfaa89ab8f3f")
	require.Contains(t, text, "repository: https://github.com/VictoriaMetrics/VictoriaMetrics")
	require.Contains(t, text, "tag: v${{package.version}}")
	require.Contains(t, text, "expected-commit: 3ac6505d7ff2c02b1c7878d92525e7b681e9cadc")
	require.Contains(t, text, "go-package: go-1.26")
	require.Contains(t, text, "GOTOOLCHAIN: local")
	require.Contains(t, text, "- nodejs-22")
	require.NotContains(t, text, "- nodejs-20")
	require.Contains(t, text, "go version -m /usr/bin/victoria-metrics")
	require.Contains(t, text, "- apk-tools")
	require.Contains(t, text, "api/v1/import/prometheus")
	require.Contains(t, text, "timestamp=$(date +%s)")
	require.Contains(t, text, "--data \"integer_victoriametrics_smoke{source=\\\"integer\\\"} 7 $timestamp\"")
	require.Contains(t, text, "internal/force_flush")
	require.Contains(t, text, "prometheus/api/v1/query")
	require.Contains(t, text, "--data-urlencode \"time=$timestamp\"")
	require.Contains(t, text, "-search.latencyOffset=0")
	require.Contains(t, text, "curl --fail-with-body --silent --show-error")
	require.Contains(t, text, "apk info --license victoriametrics | grep -Fx Apache-2.0")
	require.Contains(t, text, "usr/share/licenses/${{package.name}}/LICENSE")
	require.NotContains(t, text, "subpackages:")
}

func Test_VictoriaMetrics_resolves_only_fixed_local_package(t *testing.T) {
	// Given: the checked-in image template and locally built VictoriaMetrics package.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "victoriametrics.yaml"))
	require.NoError(t, err)
	tmpl := def.Types["default"]

	// When: the local Melange artifact is pinned for the image build.
	err = pinLocalPackageVersions(&tmpl, "1", []apkindex.Package{{
		Name:    "victoriametrics",
		Version: "1.147.0-r2",
	}})

	// Then: apko can only select the approved fixed local revision.
	require.NoError(t, err)
	require.Equal(t, []string{"victoriametrics=1.147.0-r2@local"}, tmpl.Packages)
}
