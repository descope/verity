package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/integer/apkindex"
	intconfig "github.com/verity-org/verity/internal/integer/config"
)

func Test_Velero_uses_fixed_bespoke_package_for_default_variant(t *testing.T) {
	// Given: the checked-in Velero image definition.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "velero.yaml"))
	require.NoError(t, err)

	// When: the supported image variant is resolved.
	tmpl := def.Types["default"]

	// Then: it selects only the local Velero rebuild and preserves the chart-compatible runtime.
	require.NotNil(t, tmpl.Melange)
	require.Equal(t, "velero.yaml", tmpl.Melange.Bespoke.First())
	require.Equal(t, []string{"velero"}, tmpl.Packages)
	require.Equal(t, "/usr/bin/velero", tmpl.Entrypoint)
	require.Contains(t, tmpl.Paths, intconfig.PathDef{Path: "/velero", Type: "symlink", Source: "/usr/bin/velero"})
}

func Test_Velero_recipe_pins_immutable_fixed_source_and_runtime_contract(t *testing.T) {
	// Given: the bespoke package recipe selected by the image.
	recipe, err := os.ReadFile(filepath.Join("..", "packages", "bespoke", "velero.yaml"))
	require.NoError(t, err)
	text := string(recipe)

	// When: its source, dependency, smoke, and license metadata are inspected.

	// Then: revision r4 owns Apache-2.0 v1.18.2 and both dependency security fixes.
	require.Contains(t, text, "name: velero")
	require.Contains(t, text, "# renovate: datasource=github-tags depName=velero-io/velero versioning=semver-coerced")
	require.Contains(t, text, "version: \"1.18.2\"")
	require.Contains(t, text, "epoch: 4")
	require.Contains(t, text, "license: Apache-2.0")
	require.Contains(t, text, "Adapted from wolfi-dev/os@080b7b160597fec7f7bfec73b8f281bb55c17117")
	require.Contains(t, text, "repository: https://github.com/velero-io/velero")
	require.Contains(t, text, "tag: v${{package.version}}")
	require.Contains(t, text, "expected-commit: c253c7fe37d78c9b7e55c68544f7c5b2608712d8")
	require.Contains(t, text, "go.opentelemetry.io/otel@v1.44.0")
	require.Contains(t, text, "go.opentelemetry.io/otel/metric@v1.44.0")
	require.Contains(t, text, "go.opentelemetry.io/otel/sdk@v1.44.0")
	require.Contains(t, text, "go.opentelemetry.io/otel/sdk/metric@v1.44.0")
	require.Contains(t, text, "go.opentelemetry.io/otel/trace@v1.44.0")
	require.Contains(t, text, "golang.org/x/text@v0.39.0")
	require.Contains(t, text, "go-package: go-1.26")
	require.Contains(t, text, "version --client-only")
	require.Contains(t, text, "velero backup create --help")
	require.Contains(t, text, "velero install --help")
	require.Contains(t, text, "velero completion bash")
	require.Contains(t, text, "apk info --license velero | grep -Fx Apache-2.0")
	require.Contains(t, text, "usr/share/licenses/${{package.name}}/LICENSE")
}

func Test_Velero_resolves_only_fixed_local_package(t *testing.T) {
	// Given: the checked-in image template and locally built Velero package.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "velero.yaml"))
	require.NoError(t, err)
	tmpl := def.Types["default"]

	// When: the local Melange artifact is pinned for the image build.
	err = pinLocalPackageVersions(&tmpl, "1", []apkindex.Package{{
		Name:    "velero",
		Version: "1.18.2-r4",
	}})

	// Then: apko can only select the approved fixed local revision.
	require.NoError(t, err)
	require.Equal(t, []string{"velero=1.18.2-r4@local"}, tmpl.Packages)
}
