package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/integer/apkindex"
	intconfig "github.com/verity-org/verity/internal/integer/config"
)

func Test_OpenBao_uses_pinned_bespoke_package(t *testing.T) {
	// Given: the checked-in OpenBao image definition.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "openbao.yaml"))
	require.NoError(t, err)

	// When: the approved default image variant is resolved.
	tmpl := def.Types["default"]

	// Then: the image selects the local rebuild and its runnable binary.
	require.NotNil(t, tmpl.Melange)
	require.Equal(t, "openbao.yaml", tmpl.Melange.Bespoke.First())
	require.Equal(t, []string{"openbao"}, tmpl.Packages)
	require.Equal(t, "/usr/bin/bao server", tmpl.Entrypoint)
	require.Contains(t, def.Versions, "2")
}

func Test_OpenBao_recipe_pins_fixed_source_revision(t *testing.T) {
	// Given: the bespoke package recipe selected by the image.
	recipe, err := os.ReadFile(filepath.Join("..", "packages", "bespoke", "openbao.yaml"))
	require.NoError(t, err)
	text := string(recipe)

	// When: its package, source, build, test, and license metadata are inspected.

	// Then: revision r2 builds immutable MPL-2.0 v2.5.5 source with the remediated toolchain.
	require.Contains(t, text, "name: openbao")
	require.Contains(t, text, "# renovate: datasource=github-tags depName=openbao/openbao versioning=semver-coerced")
	require.Contains(t, text, "version: \"2.5.5\"")
	require.Contains(t, text, "epoch: 2")
	require.Contains(t, text, "license: MPL-2.0")
	require.Contains(t, text, "Adapted from wolfi-dev/os@bc7cfd54e91daae028cc3411d5c6f9b5fa8483ea")
	require.Contains(t, text, "028992583c693c4de6350b8aa52ff85e30375a99.tar.gz")
	require.Contains(t, text, "expected-sha256: 92372e664f9a03968e619023cf49cf7ac8fc954c3314b545e0e1af69038ab28e")
	require.Contains(t, text, "go-package: go-1.26")
	require.Contains(t, text, "make ember-dist")
	require.Contains(t, text, "output: bao")
	require.Contains(t, text, "version.fullVersion=${{package.version}}")
	require.Contains(t, text, "version.GitCommit=028992583c693c4de6350b8aa52ff85e30375a99")
	require.Contains(t, text, "usr/share/licenses/${{package.name}}/LICENSE")
	require.Contains(t, text, "${{targets.destdir}}/usr/bin/bao\" server -dev -dev-listen-address=127.0.0.1:18200")
	require.Contains(t, text, "BAO_ADDR=http://127.0.0.1:18200")
	require.Equal(t, 2, strings.Count(text, "curl --fail-with-body --silent --show-error"))
	require.Contains(t, text, "apk info --license openbao")
	require.Contains(t, text, "bao server -dev")
	require.Contains(t, text, "bao status")
}

func Test_OpenBao_resolves_fixed_local_package(t *testing.T) {
	// Given: the checked-in image template and the package produced by the bespoke recipe.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "openbao.yaml"))
	require.NoError(t, err)
	tmpl := def.Types["default"]

	// When: the local Melange artifact is pinned for the image build.
	err = pinLocalPackageVersions(&tmpl, "2", []apkindex.Package{{
		Name:    "openbao",
		Version: "2.5.5-r2",
	}})

	// Then: apko can only select the approved fixed locally built revision.
	require.NoError(t, err)
	require.Equal(t, []string{"openbao=2.5.5-r2@local"}, tmpl.Packages)
}
