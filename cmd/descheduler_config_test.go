package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/integer/apkindex"
	intconfig "github.com/verity-org/verity/internal/integer/config"
)

func Test_Descheduler_uses_pinned_bespoke_package(t *testing.T) {
	// Given: the checked-in Descheduler image definition.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "descheduler.yaml"))
	require.NoError(t, err)

	// When: the approved default image variant is resolved.
	tmpl := def.Types["default"]

	// Then: the image selects the locally rebuilt package and preserves its runtime contract.
	require.NotNil(t, tmpl.Melange)
	require.Equal(t, "descheduler.yaml", tmpl.Melange.Bespoke.First())
	require.Equal(t, []string{"descheduler"}, tmpl.Packages)
	require.Equal(t, "/usr/bin/descheduler", tmpl.Entrypoint)
}

func Test_Descheduler_recipe_pins_fixed_source_revision(t *testing.T) {
	// Given: the bespoke package recipe selected by the image.
	recipe, err := os.ReadFile(filepath.Join("..", "packages", "bespoke", "descheduler.yaml"))
	require.NoError(t, err)
	text := string(recipe)

	// When: its package, source, dependency, and license metadata are inspected.

	// Then: approved revision r6 is rebuilt from immutable Apache-2.0 v0.36.0 source.
	require.Contains(t, text, "name: descheduler")
	require.Contains(t, text, "version: \"0.36.0\"")
	require.Contains(t, text, "epoch: 6")
	require.Contains(t, text, "license: Apache-2.0")
	require.Contains(t, text, "Adapted from wolfi-dev/os@90987ee0b29e578acc95eeb1f7efdae35d30d569")
	require.Contains(t, text, "repository: https://github.com/kubernetes-sigs/descheduler")
	require.Contains(t, text, "tag: v${{package.version}}")
	require.Contains(t, text, "expected-commit: 8005cdf78391e2c071cfa653c462dfefc2de8856")
	require.Contains(t, text, "uses: go/bump")
	require.Contains(t, text, "golang.org/x/net@v0.55.0")
	require.Contains(t, text, "go-package: go-1.26")
	require.Contains(t, text, "packages: ./cmd/descheduler")
	require.Contains(t, text, "usr/share/licenses/${{package.name}}/LICENSE")
}

func Test_Descheduler_resolves_fixed_local_package(t *testing.T) {
	// Given: the checked-in image template and the package produced by the bespoke recipe.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "descheduler.yaml"))
	require.NoError(t, err)
	tmpl := def.Types["default"]

	// When: local Melange artifacts are pinned for the image build.
	err = pinLocalPackageVersions(&tmpl, "0", []apkindex.Package{{
		Name:    "descheduler",
		Version: "0.36.0-r6",
	}})

	// Then: apko can only select the approved fixed locally built revision.
	require.NoError(t, err)
	require.Equal(t, []string{"descheduler=0.36.0-r6@local"}, tmpl.Packages)
}
