package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/integer/apkindex"
	intconfig "github.com/verity-org/verity/internal/integer/config"
)

func Test_Karpenter_uses_pinned_bespoke_package(t *testing.T) {
	// Given: the checked-in Karpenter 1.11 image definition.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "karpenter.yaml"))
	require.NoError(t, err)

	// When: the approved default image variant is resolved.
	tmpl := def.Types["default"]

	// Then: the image selects the locally rebuilt package and preserves its runtime contract.
	require.NotNil(t, tmpl.Melange)
	require.Equal(t, "karpenter-1.11.yaml", tmpl.Melange.Bespoke.First())
	require.Equal(t, []string{"karpenter-1.11"}, tmpl.Packages)
	require.Equal(t, "/usr/bin/controller", tmpl.Entrypoint)
}

func Test_Karpenter_recipe_pins_fixed_source_revision(t *testing.T) {
	// Given: the bespoke package recipe selected by the image.
	recipe, err := os.ReadFile(filepath.Join("..", "packages", "bespoke", "karpenter-1.11.yaml"))
	require.NoError(t, err)
	text := string(recipe)

	// When: its package, source, build, test, and license metadata are inspected.

	// Then: approved revision r7 rebuilds immutable Apache-2.0 v1.11.1 source with fixed Go.
	require.Contains(t, text, "name: karpenter-1.11")
	require.Contains(t, text, "version: \"1.11.1\"")
	require.Contains(t, text, "epoch: 7")
	require.Contains(t, text, "license: Apache-2.0")
	require.Contains(t, text, "Adapted from wolfi-dev/os@2c0385b38bd6869a0e928363916e7a7148a1acaf")
	require.Contains(t, text, "77c161351fbad4f23fae329412de22f1045ccb42.tar.gz")
	require.Contains(t, text, "expected-sha256: 73ee8d8865e0bddbe963923172c894212c4775063c5bebdfea7535ec2e6812c7")
	require.Contains(t, text, "go-package: go-1.26")
	require.Contains(t, text, "packages: ./cmd/controller")
	require.Contains(t, text, "output: controller")
	require.Contains(t, text, "Version=${{package.version}}")
	require.Contains(t, text, "/usr/bin/controller --help")
	require.Contains(t, text, "go version -m /usr/bin/controller")
	require.Contains(t, text, "apk info --who-owns /usr/bin/controller")
	require.Contains(t, text, "usr/share/licenses/${{package.name}}/LICENSE")
}

func Test_Karpenter_resolves_fixed_local_package(t *testing.T) {
	// Given: the checked-in image template and the package produced by the bespoke recipe.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "karpenter.yaml"))
	require.NoError(t, err)
	tmpl := def.Types["default"]

	// When: local Melange artifacts are pinned for the image build.
	err = pinLocalPackageVersions(&tmpl, "1.11", []apkindex.Package{{
		Name:    "karpenter-1.11",
		Version: "1.11.1-r7",
	}})

	// Then: apko can only select the approved fixed locally built revision.
	require.NoError(t, err)
	require.Equal(t, []string{"karpenter-1.11=1.11.1-r7@local"}, tmpl.Packages)
}
