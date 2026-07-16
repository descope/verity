package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/integer/apkindex"
	intconfig "github.com/verity-org/verity/internal/integer/config"
)

func Test_Pluto_uses_pinned_bespoke_package(t *testing.T) {
	// Given: the checked-in Pluto image definition.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "pluto.yaml"))
	require.NoError(t, err)

	// When: the approved default image variant is resolved.
	tmpl := def.Types["default"]

	// Then: the image selects the local rebuild and preserves its runtime contract.
	require.NotNil(t, tmpl.Melange)
	require.Equal(t, "pluto.yaml", tmpl.Melange.Bespoke.First())
	require.Equal(t, []string{"pluto"}, tmpl.Packages)
	require.Equal(t, "/usr/bin/pluto", tmpl.Entrypoint)
	require.Contains(t, def.Versions, "5")
}

func Test_Pluto_recipe_pins_fixed_source_revision(t *testing.T) {
	// Given: the bespoke package recipe selected by the image.
	recipe, err := os.ReadFile(filepath.Join("..", "packages", "bespoke", "pluto.yaml"))
	require.NoError(t, err)
	text := string(recipe)

	// When: its package, source, build, test, and license metadata are inspected.

	// Then: revision r9 rebuilds immutable Apache-2.0 v5.24.0 source with a patched Go toolchain.
	require.Contains(t, text, "name: pluto")
	require.Contains(t, text, "# renovate: datasource=github-tags depName=FairwindsOps/pluto versioning=semver-coerced")
	require.Contains(t, text, "version: \"5.24.0\"")
	require.Contains(t, text, "epoch: 9")
	require.Contains(t, text, "license: Apache-2.0")
	require.Contains(t, text, "Adapted from wolfi-dev/os@3c0cdd0d7d4d2933e3aa3cae7bde1c87319a9151")
	require.Contains(t, text, "dd5ec8cccce5e42dfe8054b8250baa35546056a0.tar.gz")
	require.Contains(t, text, "expected-sha256: 3abb9e72670f7466157b225ccdf68004122b4cefa54d9b7a9af687bb810cffc2")
	require.Contains(t, text, "go-package: go-1.26")
	require.Contains(t, text, "go get golang.org/x/net@v0.55.0")
	require.Contains(t, text, "packages: ./cmd/pluto/main.go")
	require.Contains(t, text, "output: pluto")
	require.Contains(t, text, "main.version=${{package.version}}")
	require.Contains(t, text, "main.commit=dd5ec8cccce5e42dfe8054b8250baa35546056a0")
	require.Contains(t, text, "usr/share/licenses/${{package.name}}/LICENSE")
	require.Contains(t, text, "pluto detect")
	require.Contains(t, text, "apk info --who-owns /usr/bin/pluto")
}

func Test_Pluto_resolves_fixed_local_package(t *testing.T) {
	// Given: the checked-in image template and the package produced by the bespoke recipe.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "pluto.yaml"))
	require.NoError(t, err)
	tmpl := def.Types["default"]

	// When: the local Melange artifact is pinned for the image build.
	err = pinLocalPackageVersions(&tmpl, "5", []apkindex.Package{{
		Name:    "pluto",
		Version: "5.24.0-r9",
	}})

	// Then: apko can only select the approved fixed locally built revision.
	require.NoError(t, err)
	require.Equal(t, []string{"pluto=5.24.0-r9@local"}, tmpl.Packages)
}
