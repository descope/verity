package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/integer/apkindex"
	intconfig "github.com/verity-org/verity/internal/integer/config"
)

func Test_KubeBench_uses_pinned_bespoke_packages(t *testing.T) {
	// Given: the checked-in kube-bench image definition.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "kube-bench.yaml"))
	require.NoError(t, err)

	// When: the approved default image variant is resolved.
	tmpl := def.Types["default"]

	// Then: the image selects the local rebuild and preserves its runtime contract.
	require.NotNil(t, tmpl.Melange)
	require.Equal(t, "kube-bench.yaml", tmpl.Melange.Bespoke.First())
	require.Equal(t, []string{"kube-bench", "kube-bench-configs"}, tmpl.Packages)
	require.Equal(t, "/usr/bin/kube-bench", tmpl.Entrypoint)
}

func Test_KubeBench_recipe_pins_fixed_source_revision(t *testing.T) {
	// Given: the bespoke package recipe selected by the image.
	recipe, err := os.ReadFile(filepath.Join("..", "packages", "bespoke", "kube-bench.yaml"))
	require.NoError(t, err)
	text := string(recipe)

	// When: its package, source, dependency, build, test, and license metadata are inspected.

	// Then: approved revision r16 rebuilds immutable Apache-2.0 v0.15.0 source with fixed Go dependencies.
	require.Contains(t, text, "name: kube-bench")
	require.Contains(t, text, "version: \"0.15.0\"")
	require.Contains(t, text, "epoch: 16")
	require.Contains(t, text, "license: Apache-2.0")
	require.Contains(t, text, "Adapted from wolfi-dev/os@f27a9a5190075207acb9d5244fa982b58423e812")
	require.Contains(t, text, "8a7204d42f0bea44db453c545db052c29f58a9cc.tar.gz")
	require.Contains(t, text, "expected-sha256: 47dd0f0c95134c103bbd1dae7ccf4295fc343f0ebb6ba9aac1184a2c781a5e58")
	require.Contains(t, text, "github.com/jackc/pgx/v5@v5.9.2")
	require.Contains(t, text, "golang.org/x/net@v0.55.0")
	require.Contains(t, text, "golang.org/x/text@v0.39.0")
	require.Contains(t, text, "go-package: go-1.26")
	require.Contains(t, text, "packages: .")
	require.Contains(t, text, "output: kube-bench")
	require.Contains(t, text, "KubeBenchVersion=v${{package.version}}")
	require.Contains(t, text, "name: \"kube-bench-configs\"")
	require.Contains(t, text, "usr/share/licenses/${{package.name}}/LICENSE")
	require.Contains(t, text, "kube-bench version")
	require.Contains(t, text, "apk info --who-owns /usr/bin/kube-bench")
}

func Test_KubeBench_resolves_fixed_local_packages(t *testing.T) {
	// Given: the checked-in image template and packages produced by the bespoke recipe.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "kube-bench.yaml"))
	require.NoError(t, err)
	tmpl := def.Types["default"]

	// When: local Melange artifacts are pinned for the image build.
	err = pinLocalPackageVersions(&tmpl, "0", []apkindex.Package{
		{Name: "kube-bench", Version: "0.15.0-r16"},
		{Name: "kube-bench-configs", Version: "0.15.0-r16"},
	})

	// Then: apko can only select the approved fixed locally built revisions.
	require.NoError(t, err)
	require.Equal(t, []string{
		"kube-bench=0.15.0-r16@local",
		"kube-bench-configs=0.15.0-r16@local",
	}, tmpl.Packages)
}
