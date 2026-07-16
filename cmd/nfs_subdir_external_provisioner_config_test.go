package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/integer/apkindex"
	intconfig "github.com/verity-org/verity/internal/integer/config"
)

func Test_NFSSubdirExternalProvisioner_uses_pinned_bespoke_package(t *testing.T) {
	// Given: the checked-in NFS subdir external provisioner image definition.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "nfs-subdir-external-provisioner.yaml"))
	require.NoError(t, err)

	// When: the approved default image variant is resolved.
	tmpl := def.Types["default"]

	// Then: the image selects the local rebuild and preserves its runtime contract.
	require.NotNil(t, tmpl.Melange)
	require.Equal(t, "nfs-subdir-external-provisioner.yaml", tmpl.Melange.Bespoke.First())
	require.Equal(t, []string{"nfs-subdir-external-provisioner"}, tmpl.Packages)
	require.Equal(t, "/usr/bin/nfs-subdir-external-provisioner", tmpl.Entrypoint)
	require.Contains(t, def.Versions, "4")
}

func Test_NFSSubdirExternalProvisioner_recipe_pins_fixed_source_revision(t *testing.T) {
	// Given: the bespoke package recipe selected by the image.
	recipe, err := os.ReadFile(filepath.Join("..", "packages", "bespoke", "nfs-subdir-external-provisioner.yaml"))
	require.NoError(t, err)
	text := string(recipe)

	// When: its package, source, dependency, build, test, and license metadata are inspected.

	// Then: direct revision r44 rebuilds immutable Apache-2.0 v4.0.18 source with fixed Go.
	require.Contains(t, text, "name: nfs-subdir-external-provisioner")
	require.Contains(t, text, "version: \"4.0.18\"")
	require.Contains(t, text, "epoch: 44")
	require.Contains(t, text, "license: Apache-2.0")
	require.Contains(t, text, "Adapted from wolfi-dev/os@5637ed9b389b64dbf9b76eaddcebfb09cce4e807")
	require.Contains(t, text, "c2a2d5d544781e3d3e4d7a7e2d21a8e64cf6d9d1.tar.gz")
	require.Contains(t, text, "expected-sha256: 63a2b7adc859ec25e98b705aadc402f1fa8de4e9fa9f0baab4ac372768bcd9f3")
	require.Contains(t, text, "golang.org/x/crypto@v0.52.0")
	require.Contains(t, text, "golang.org/x/net@v0.55.0")
	require.Contains(t, text, "go-package: go-1.26")
	require.Contains(t, text, "packages: ./cmd/nfs-subdir-external-provisioner")
	require.Contains(t, text, "output: nfs-subdir-external-provisioner")
	require.Contains(t, text, "usr/share/licenses/${{package.name}}/LICENSE")
	require.Contains(t, text, "nfs-subdir-external-provisioner --help")
	require.Contains(t, text, "go version -m /usr/bin/nfs-subdir-external-provisioner")
	require.Contains(t, text, "apk info --who-owns /usr/bin/nfs-subdir-external-provisioner")
	require.Contains(t, text, "PROVISIONER_NAME=test/nfs-provisioner")
	require.Contains(t, text, "Failed to create kubeconfig")
}

func Test_NFSSubdirExternalProvisioner_resolves_fixed_local_package(t *testing.T) {
	// Given: the checked-in image template and package produced by the bespoke recipe.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "nfs-subdir-external-provisioner.yaml"))
	require.NoError(t, err)
	tmpl := def.Types["default"]

	// When: the local Melange artifact is pinned for the image build.
	err = pinLocalPackageVersions(&tmpl, "4", []apkindex.Package{{
		Name:    "nfs-subdir-external-provisioner",
		Version: "4.0.18-r44",
	}})

	// Then: apko can only select the approved fixed locally built revision.
	require.NoError(t, err)
	require.Equal(t, []string{"nfs-subdir-external-provisioner=4.0.18-r44@local"}, tmpl.Packages)
}
