package cmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"

	"github.com/verity-org/verity/internal/integer/apkindex"
	intconfig "github.com/verity-org/verity/internal/integer/config"
)

func Test_MetricsServer_advertises_only_broad_alias_and_uses_pinned_bespoke_package(t *testing.T) {
	// Given: the checked-in metrics-server image definition.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "metrics-server.yaml"))
	require.NoError(t, err)

	// When: the default image variant and advertised version tracks are resolved.
	tmpl := def.Types["default"]

	// Then: only the broad alias remains and it preserves the runtime contract.
	require.Len(t, def.Versions, 1)
	require.Contains(t, def.Versions, "0")
	require.NotContains(t, def.Versions, "0.8")
	require.NotNil(t, tmpl.Melange)
	require.Equal(t, "metrics-server.yaml", tmpl.Melange.Bespoke.First())
	require.Equal(t, []string{"metrics-server"}, tmpl.Packages)
	require.Equal(t, "/usr/bin/metrics-server", tmpl.Entrypoint)
}

func Test_MetricsServer_recipe_pins_clean_upstream_release(t *testing.T) {
	// Given: the bespoke package recipe selected by the image.
	recipe, err := os.ReadFile(filepath.Join("..", "packages", "bespoke", "metrics-server.yaml"))
	require.NoError(t, err)
	text := string(recipe)

	// When: its package, source, runtime, and license metadata are inspected.

	// Then: revision r0 builds immutable Apache-2.0 v0.9.0 source with fixed dependencies.
	require.Contains(t, text, "name: metrics-server")
	require.Contains(t, text, "version: \"0.9.0\"")
	require.Contains(t, text, "epoch: 0")
	require.Contains(t, text, "license: Apache-2.0")
	require.Contains(t, text, "archive/2a7c4b2c7d46552ff47f4aeaa3a735c582587ecd.tar.gz")
	require.Contains(t, text, "expected-sha256: 7c3dc479484fca306297221058bf55f5d4620305b4041e729dfb497386a621fa")
	require.Contains(t, text, "GIT_TAG=v${{package.version}}")
	require.Contains(t, text, "GIT_COMMIT=2a7c4b2c7d46552ff47f4aeaa3a735c582587ecd")
	require.Contains(t, text, "github.com/prometheus/prometheus\\tv0.313.0")
	require.Contains(t, text, "go.opentelemetry.io/otel\\tv1.44.0")
	require.Contains(t, text, "metrics-server --version")
	require.Contains(t, text, "usr/share/licenses/${{package.name}}/LICENSE")
}

func Test_MetricsServer_broad_alias_resolves_clean_local_package(t *testing.T) {
	// Given: the checked-in image template and the package produced by the bespoke recipe.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "metrics-server.yaml"))
	require.NoError(t, err)
	tmpl := def.Types["default"]

	// When: local Melange artifacts are pinned for the surviving broad alias.
	pinErr := pinLocalPackageVersions(&tmpl, "0", []apkindex.Package{{
		Name:    "metrics-server",
		Version: "0.9.0-r0",
	}})

	// Then: apko can only select the clean locally built revision.
	require.NoError(t, pinErr)
	require.Equal(t, []string{"metrics-server=0.9.0-r0@local"}, tmpl.Packages)
}

func Test_MetricsServer_explicit_retired_alias_fails_before_build(t *testing.T) {
	// Given: an explicit request for the retired 0.8 alias with network discovery disabled.
	output := filepath.Join(t.TempDir(), "metrics-server.tar")
	root := &cli.Command{Commands: []*cli.Command{IntegerCommand}}

	// When: the local image build command validates the requested version.
	err := root.Run(context.Background(), []string{
		"verity", "integer", "build",
		"--image", "metrics-server",
		"--version", "0.8",
		"--type", "default",
		"--images-dir", filepath.Join("..", "images"),
		"--apkindex-url", "",
		"--output", output,
	})

	// Then: it rejects the unsupported version before producing an image.
	require.ErrorIs(t, err, errIntegerVariantNotFound)
	require.Contains(t, err.Error(), `image "metrics-server" version "0.8" not defined for build`)
	require.NoFileExists(t, output)
}
