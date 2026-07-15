package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/integer/apkindex"
	intconfig "github.com/verity-org/verity/internal/integer/config"
)

func Test_Dex_uses_pinned_bespoke_package(t *testing.T) {
	// Given: the checked-in Dex image definition.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "dex.yaml"))
	require.NoError(t, err)

	// When: the approved default image variant is resolved.
	tmpl := def.Types["default"]

	// Then: the image selects the locally rebuilt package without changing the chart-driven argv contract.
	require.NotNil(t, tmpl.Melange)
	require.Equal(t, "dex.yaml", tmpl.Melange.Bespoke.First())
	require.Equal(t, []string{"dex"}, tmpl.Packages)
	require.Empty(t, tmpl.Entrypoint)
}

func Test_Dex_recipe_pins_fixed_source_revision(t *testing.T) {
	// Given: the bespoke package recipe selected by the image.
	recipe, err := os.ReadFile(filepath.Join("..", "packages", "bespoke", "dex.yaml"))
	require.NoError(t, err)
	text := string(recipe)

	// When: its package, source, dependency, build, and license metadata are inspected.

	// Then: approved revision r15 is rebuilt from immutable Apache-2.0 v2.45.1 source.
	require.Contains(t, text, "name: dex")
	require.Contains(t, text, "version: \"2.45.1\"")
	require.Contains(t, text, "epoch: 15")
	require.Contains(t, text, "license: Apache-2.0")
	require.Contains(t, text, "Adapted from wolfi-dev/os@e3f029ef29e67d4d8d9f041b1ae994719d96161f")
	require.Contains(t, text, "11d2eeb52b42e1980e14cb91e69dd9e3faab2076.tar.gz")
	require.Contains(t, text, "expected-sha256: 18bf92e8ccbf53e86814c2beb39b7d59f28fb07c84639e9f47a9bb5ea764e0b9")
	require.Contains(t, text, "golang.org/x/net@v0.55.0")
	require.Contains(t, text, "golang.org/x/crypto@v0.52.0")
	require.Contains(t, text, "go-package: go-1.26")
	require.Contains(t, text, "packages: ./cmd/dex")
	require.Contains(t, text, "output: dex")
	require.Contains(t, text, "usr/bin/docker-entrypoint")
	require.Contains(t, text, "usr/share/licenses/${{package.name}}/LICENSE")
	require.NotContains(t, text, "\"${{targets.destdir}}/var/dex\"")
}

func Test_Dex_resolves_fixed_local_package(t *testing.T) {
	// Given: the checked-in image template and the package produced by the bespoke recipe.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "dex.yaml"))
	require.NoError(t, err)
	tmpl := def.Types["default"]

	// When: local Melange artifacts are pinned for the image build.
	err = pinLocalPackageVersions(&tmpl, "2", []apkindex.Package{{
		Name:    "dex",
		Version: "2.45.1-r15",
	}})

	// Then: apko can only select the approved fixed locally built revision.
	require.NoError(t, err)
	require.Equal(t, []string{"dex=2.45.1-r15@local"}, tmpl.Packages)
}
