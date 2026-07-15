package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/integer/apkindex"
	intconfig "github.com/verity-org/verity/internal/integer/config"
)

func Test_KOR_uses_pinned_bespoke_package(t *testing.T) {
	// Given: the checked-in KOR image definition.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "kor.yaml"))
	require.NoError(t, err)

	// When: the approved default image variant is resolved.
	tmpl := def.Types["default"]

	// Then: the image selects the locally rebuilt package and preserves its runtime contract.
	require.NotNil(t, tmpl.Melange)
	require.Equal(t, "kor.yaml", tmpl.Melange.Bespoke.First())
	require.Equal(t, []string{"kor"}, tmpl.Packages)
	require.Equal(t, "/usr/bin/kor", tmpl.Entrypoint)
}

func Test_KOR_recipe_pins_fixed_source_revision(t *testing.T) {
	// Given: the bespoke package recipe selected by the image.
	recipe, err := os.ReadFile(filepath.Join("..", "packages", "bespoke", "kor.yaml"))
	require.NoError(t, err)
	text := string(recipe)

	// When: its package, source, dependency, build, and license metadata are inspected.

	// Then: approved revision r8 rebuilds immutable MIT-licensed v0.6.8 source with the fixed toolchain.
	require.Contains(t, text, "name: kor")
	require.Contains(t, text, "version: \"0.6.8\"")
	require.Contains(t, text, "epoch: 8")
	require.Contains(t, text, "license: MIT")
	require.Contains(t, text, "Adapted from wolfi-dev/os@86f2c8a880f339829deb63a993f563382356221e")
	require.Contains(t, text, "d6986c937ce72d331fef965c1b103fab69b4ea93.tar.gz")
	require.Contains(t, text, "expected-sha256: dce84e8a1d5de7311a5ddf3bd31c443f177f3ea96d182757fd4190e5b484bdfa")
	require.Contains(t, text, "golang.org/x/net@v0.55.0")
	require.Contains(t, text, "go-package: go-1.26")
	require.Contains(t, text, "packages: .")
	require.Contains(t, text, "output: kor")
	require.Contains(t, text, "github.com/yonahd/kor/pkg/utils.Version=${{package.version}}")
	require.Contains(t, text, "\"${{targets.destdir}}/usr/bin/kor\" version")
	require.Contains(t, text, "\"${{targets.destdir}}/usr/bin/kor\" --help")
	require.Contains(t, text, "/usr/bin/kor version")
	require.Contains(t, text, "apk info --who-owns /usr/bin/kor")
	require.Contains(t, text, "usr/share/licenses/${{package.name}}/LICENSE")
}

func Test_KOR_resolves_fixed_local_package(t *testing.T) {
	// Given: the checked-in image template and the package produced by the bespoke recipe.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "kor.yaml"))
	require.NoError(t, err)
	tmpl := def.Types["default"]

	// When: local Melange artifacts are pinned for the image build.
	err = pinLocalPackageVersions(&tmpl, "0", []apkindex.Package{{
		Name:    "kor",
		Version: "0.6.8-r8",
	}})

	// Then: apko can only select the approved fixed locally built revision.
	require.NoError(t, err)
	require.Equal(t, []string{"kor=0.6.8-r8@local"}, tmpl.Packages)
}
