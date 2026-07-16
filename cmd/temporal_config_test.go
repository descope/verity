package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/integer/apkindex"
	intconfig "github.com/verity-org/verity/internal/integer/config"
)

func Test_Temporal_uses_pinned_bespoke_package_with_runnable_default(t *testing.T) {
	// Given: the checked-in Temporal image definition.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "temporal.yaml"))
	require.NoError(t, err)

	// When: the approved default image variant is resolved.
	tmpl := def.Types["default"]

	// Then: it uses the local rebuild and starts the self-contained SQLite development server.
	require.NotNil(t, tmpl.Melange)
	require.Equal(t, "temporal-server.yaml", tmpl.Melange.Bespoke.First())
	require.Equal(t, []string{"temporal-server"}, tmpl.Packages)
	require.Equal(t, "/usr/bin/temporal-server", tmpl.Entrypoint)
	require.Equal(t, "--config-file config/development-sqlite.yaml --allow-no-auth start", tmpl.Cmd)
	require.Equal(t, "/etc/temporal", tmpl.WorkDir)
}

func Test_Temporal_recipe_pins_immutable_source_and_runtime_contract(t *testing.T) {
	// Given: the bespoke package recipe selected by the image.
	recipe, err := os.ReadFile(filepath.Join("..", "packages", "bespoke", "temporal-server.yaml"))
	require.NoError(t, err)
	text := string(recipe)

	// When: its package, source, build, runtime, health, and license metadata are inspected.

	// Then: it rebuilds the approved MIT-licensed Temporal 1.31.2 release from its immutable commit.
	require.Contains(t, text, "name: temporal-server")
	require.Contains(t, text, "version: \"1.31.2\"")
	require.Contains(t, text, "epoch: 1")
	require.Contains(t, text, "license: MIT")
	require.Contains(t, text, "repository: https://github.com/temporalio/temporal")
	require.Contains(t, text, "tag: v${{package.version}}")
	require.Contains(t, text, "expected-commit: 19a774302c613da9adc4436ab14278ccdca8e0a5")
	require.Contains(t, text, "go-package: go-1.26")
	require.Contains(t, text, "go.opentelemetry.io/otel@v1.44.0")
	require.Contains(t, text, "packages: ./cmd/server")
	require.Contains(t, text, "output: temporal-server")
	require.Contains(t, text, "config/development-sqlite.yaml")
	require.Contains(t, text, "config/dynamicconfig/development-sql.yaml")
	require.Contains(t, text, "temporal-server --help")
	require.Contains(t, text, "test/daemon-check-output")
	require.Contains(t, text, "ln -sfn /etc/temporal/config config")
	require.Contains(t, text, "/usr/bin/temporal-server --config-file /etc/temporal/config/development-sqlite.yaml")
	require.Contains(t, text, "Frontend is now healthy")
	require.Contains(t, text, "127.0.0.1:7233")
	require.Contains(t, text, "usr/share/licenses/${{package.name}}/LICENSE")
}

func Test_Temporal_resolves_fixed_local_package(t *testing.T) {
	// Given: the checked-in image template and locally built Temporal package.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "temporal.yaml"))
	require.NoError(t, err)
	tmpl := def.Types["default"]

	// When: the local Melange artifact is pinned for the image build.
	err = pinLocalPackageVersions(&tmpl, "1", []apkindex.Package{{
		Name:    "temporal-server",
		Version: "1.31.2-r1",
	}})

	// Then: apko can only select the approved locally built revision.
	require.NoError(t, err)
	require.Equal(t, []string{"temporal-server=1.31.2-r1@local"}, tmpl.Packages)
}
