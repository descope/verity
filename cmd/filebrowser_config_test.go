package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/integer/apkindex"
	intconfig "github.com/verity-org/verity/internal/integer/config"
)

func Test_FileBrowser_uses_pinned_bespoke_package(t *testing.T) {
	// Given: the checked-in File Browser image definition.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "filebrowser.yaml"))
	require.NoError(t, err)

	// When: the approved default image variant is resolved.
	tmpl := def.Types["default"]

	// Then: the image selects the locally rebuilt package and preserves its runtime contract.
	require.NotNil(t, tmpl.Melange)
	require.Equal(t, "filebrowser.yaml", tmpl.Melange.Bespoke.First())
	require.Equal(t, []string{"filebrowser"}, tmpl.Packages)
	require.Equal(t, "/usr/bin/filebrowser", tmpl.Entrypoint)
	require.Len(t, tmpl.Paths, 2)
}

func Test_FileBrowser_recipe_pins_fixed_source_revision(t *testing.T) {
	// Given: the bespoke package recipe selected by the image.
	recipe, err := os.ReadFile(filepath.Join("..", "packages", "bespoke", "filebrowser.yaml"))
	require.NoError(t, err)
	text := string(recipe)

	// When: its package, source, build, test, and license metadata are inspected.

	// Then: approved revision r1 rebuilds immutable Apache-2.0 v2.63.18 source.
	require.Contains(t, text, "name: filebrowser")
	require.Contains(t, text, "version: \"2.63.18\"")
	require.Contains(t, text, "epoch: 1")
	require.Contains(t, text, "license: Apache-2.0")
	require.Contains(t, text, "Adapted from wolfi-dev/os@6993e8258528d998f90581d5f9f97cbfdbe17c3f")
	require.Contains(t, text, "https://github.com/filebrowser/filebrowser/archive/refs/tags/v${{package.version}}.tar.gz")
	require.Contains(t, text, "expected-sha256: b665942fa4adb882498e89f09ef90f4690de4f22de043453c28e201c812060c5")
	require.Contains(t, text, "node-version: 24")
	require.Contains(t, text, "pnpm install --frozen-lockfile")
	require.Contains(t, text, "packages: .")
	require.Contains(t, text, "output: ${{package.name}}")
	require.Contains(t, text, "Version=${{package.version}}")
	require.NotContains(t, text, "Version=v${{package.version}}")
	require.Contains(t, text, "CommitSHA=fe7efb2e6afe66774cd86a5b0a03033bd514d0c0")
	require.Contains(t, text, "filebrowser version")
	require.Contains(t, text, "curl -sf http://localhost:8080/health")
	require.Contains(t, text, "usr/share/licenses/${{package.name}}/LICENSE")
}

func Test_FileBrowser_resolves_fixed_local_package(t *testing.T) {
	// Given: the checked-in image template and the package produced by the bespoke recipe.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "filebrowser.yaml"))
	require.NoError(t, err)
	tmpl := def.Types["default"]

	// When: local Melange artifacts are pinned for the image build.
	err = pinLocalPackageVersions(&tmpl, "0", []apkindex.Package{{
		Name:    "filebrowser",
		Version: "2.63.18-r1",
	}})

	// Then: apko can only select the approved fixed locally built revision.
	require.NoError(t, err)
	require.Equal(t, []string{"filebrowser=2.63.18-r1@local"}, tmpl.Packages)
}
