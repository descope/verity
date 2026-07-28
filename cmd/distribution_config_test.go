package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/integer/apkindex"
	intconfig "github.com/verity-org/verity/internal/integer/config"
)

func Test_Distribution_uses_pinned_bespoke_package(t *testing.T) {
	// Given: the checked-in CNCF Distribution image definition.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "distribution.yaml"))
	require.NoError(t, err)

	// When: the approved default image variant is resolved.
	tmpl := def.Types["default"]

	// Then: the image selects the locally rebuilt package and preserves its runtime contract.
	require.NotNil(t, tmpl.Melange)
	require.Equal(t, "distribution.yaml", tmpl.Melange.Bespoke.First())
	require.Equal(t, []string{"distribution"}, tmpl.Packages)
	require.Equal(t, "/usr/bin/registry serve /etc/docker/registry/config.yml", tmpl.Entrypoint)
	require.Len(t, tmpl.Paths, 1)
	require.Equal(t, "/var/lib/registry", tmpl.Paths[0].Path)
	require.Equal(t, "directory", tmpl.Paths[0].Type)
	require.Equal(t, 65532, tmpl.Paths[0].UID)
	require.Equal(t, 65532, tmpl.Paths[0].GID)
	require.Equal(t, "0o755", tmpl.Paths[0].Permissions)
}

func Test_Distribution_recipe_pins_fixed_source_revision(t *testing.T) {
	// Given: the bespoke package recipe selected by the image.
	recipe, err := os.ReadFile(filepath.Join("..", "packages", "bespoke", "distribution.yaml"))
	require.NoError(t, err)
	text := string(recipe)

	// When: its package, source, dependency, and license metadata are inspected.

	// Then: approved revision r9 is rebuilt from immutable Apache-2.0 v3.1.1 source.
	require.Contains(t, text, "name: distribution")
	require.Contains(t, text, "version: \"3.1.1\"")
	require.Contains(t, text, "epoch: 9")
	require.Contains(t, text, "license: Apache-2.0")
	require.Contains(t, text, "Adapted from wolfi-dev/os@572ee9ed3e434cc0991a8b6967873896381e76b8")
	require.Contains(t, text, "repository: https://github.com/distribution/distribution")
	require.Contains(t, text, "tag: v${{package.version}}")
	require.Contains(t, text, "expected-commit: 9a8d98b679740cd514aa7e7d84d23d442a5ef54c")
	require.Contains(t, text, "uses: go/bump")
	require.Contains(t, text, "golang.org/x/net@v0.55.0")
	require.Contains(t, text, "golang.org/x/crypto@v0.51.0")
	require.Contains(t, text, "golang.org/x/text@v0.39.0")
	require.Contains(t, text, "go-package: go-1.26")
	require.Contains(t, text, "packages: ./cmd/registry")
	require.Contains(t, text, "/version.version=v${{package.version}}")
	require.Contains(t, text, "/version.revision=$(git rev-parse --short HEAD)")
	require.NotContains(t, text, "/version.Version=")
	require.Contains(t, text, "registry --version")
	require.Contains(t, text, "! registry --version | grep -F \"+unknown\"")
	require.Contains(t, text, "registry --help")
	require.Contains(t, text, "usr/share/licenses/${{package.name}}/LICENSE")
	require.Contains(t, text, "var/lib/db/sbom")
	require.Contains(t, text, "chmod 0777")
	require.Contains(t, text, "\"tag\":")
	require.NotContains(t, text, "test/tw/ldd-check")
}

func Test_Distribution_resolves_fixed_local_package(t *testing.T) {
	// Given: the checked-in image template and the package produced by the bespoke recipe.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "distribution.yaml"))
	require.NoError(t, err)
	tmpl := def.Types["default"]

	// When: local Melange artifacts are pinned for the image build.
	err = pinLocalPackageVersions(&tmpl, "0", []apkindex.Package{{
		Name:    "distribution",
		Version: "3.1.1-r9",
	}})

	// Then: apko can only select the approved fixed locally built revision.
	require.NoError(t, err)
	require.Equal(t, []string{"distribution=3.1.1-r9@local"}, tmpl.Packages)
}
