package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/integer/apkindex"
	intconfig "github.com/verity-org/verity/internal/integer/config"
)

func Test_Authservice_uses_pinned_bespoke_package(t *testing.T) {
	// Given: the checked-in Authservice image definition.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "authservice.yaml"))
	require.NoError(t, err)

	// When: the default image variant is resolved.
	tmpl := def.Types["default"]

	// Then: the image selects the locally rebuilt package and preserves its runtime contract.
	require.NotNil(t, tmpl.Melange)
	require.Equal(t, "authservice.yaml", tmpl.Melange.Bespoke.First())
	require.Equal(t, []string{"authservice"}, tmpl.Packages)
	require.Equal(t, "/usr/bin/authservice", tmpl.Entrypoint)
}

func Test_Authservice_recipe_pins_fixed_source_revision(t *testing.T) {
	// Given: the bespoke package recipe selected by the image.
	recipe, err := os.ReadFile(filepath.Join("..", "packages", "bespoke", "authservice.yaml"))
	require.NoError(t, err)
	text := string(recipe)

	// When: its package and source metadata are inspected.

	// Then: revision r2 is the approved fixed release from immutable Apache-2.0 source.
	require.Contains(t, text, "name: authservice")
	require.Contains(t, text, "version: \"1.1.7\"")
	require.Contains(t, text, "epoch: 2")
	require.Contains(t, text, "license: Apache-2.0")
	require.Contains(t, text, "repository: https://github.com/istio-ecosystem/authservice")
	require.Contains(t, text, "tag: v${{package.version}}")
	require.Contains(t, text, "expected-commit: 9d868a5a9b93b49eeba72f4852164798fc794a1d")
	require.Contains(t, text, "packages: ./cmd")
	require.Contains(t, text, "github.com/tetratelabs/run/pkg/version.build")
}

func Test_Authservice_resolves_fixed_local_package(t *testing.T) {
	// Given: the checked-in image template and the package produced by the bespoke recipe.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "authservice.yaml"))
	require.NoError(t, err)
	tmpl := def.Types["default"]

	// When: local Melange artifacts are pinned for the image build.
	err = pinLocalPackageVersions(&tmpl, "1", []apkindex.Package{{
		Name:    "authservice",
		Version: "1.1.7-r2",
	}})

	// Then: apko can only select the fixed locally built revision.
	require.NoError(t, err)
	require.Equal(t, []string{"authservice=1.1.7-r2@local"}, tmpl.Packages)
}
