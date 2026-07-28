package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/integer/apkindex"
	intconfig "github.com/verity-org/verity/internal/integer/config"
)

func Test_KubeRBACProxy_uses_pinned_bespoke_package(t *testing.T) {
	// Given: the checked-in kube-rbac-proxy image definition.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "kube-rbac-proxy.yaml"))
	require.NoError(t, err)

	// When: the approved default image variant is resolved.
	tmpl := def.Types["default"]

	// Then: the image selects the local rebuild and preserves its runtime contract.
	require.NotNil(t, tmpl.Melange)
	require.Equal(t, "kube-rbac-proxy.yaml", tmpl.Melange.Bespoke.First())
	require.Equal(t, []string{"kube-rbac-proxy"}, tmpl.Packages)
	require.Equal(t, "/usr/bin/kube-rbac-proxy", tmpl.Entrypoint)
}

func Test_KubeRBACProxy_recipe_pins_fixed_source_revision(t *testing.T) {
	// Given: the bespoke package recipe selected by the image.
	recipe, err := os.ReadFile(filepath.Join("..", "packages", "bespoke", "kube-rbac-proxy.yaml"))
	require.NoError(t, err)
	text := string(recipe)

	// When: its package, source, build, test, and license metadata are inspected.

	// Then: approved revision r3 rebuilds immutable Apache-2.0 v0.22.1 source with fixed Go dependencies.
	require.Contains(t, text, "name: kube-rbac-proxy")
	require.Contains(t, text, "version: \"0.22.1\"")
	require.Contains(t, text, "epoch: 3")
	require.Contains(t, text, "license: Apache-2.0")
	require.Contains(t, text, "Adapted from wolfi-dev/os@832715bf59583579564b2fe393aba0a6b466fb85")
	require.Contains(t, text, "v0.22.1 resolves to a67fc47c9e13db72e64b3cf0a369de4350e1c59b")
	require.Contains(t, text, "https://github.com/kube-rbac-proxy/kube-rbac-proxy/archive/refs/tags/v${{package.version}}.tar.gz")
	require.Contains(t, text, "expected-sha256: f8c6d4bc140ae04f20d6d0fedb077e8c233b72bab681500d32cc07c65208c41a")
	require.Contains(t, text, "golang.org/x/text@v0.39.0")
	require.Contains(t, text, "go-package: go-1.26")
	require.Contains(t, text, "packages: ./cmd/kube-rbac-proxy")
	require.Contains(t, text, "output: kube-rbac-proxy")
	require.Contains(t, text, "gitVersion=v${{package.version}}")
	require.Contains(t, text, "kube-rbac-proxy --version")
	require.Contains(t, text, "apk info --license kube-rbac-proxy")
	require.Contains(t, text, "usr/share/licenses/${{package.name}}/LICENSE")
}

func Test_KubeRBACProxy_resolves_fixed_local_package(t *testing.T) {
	// Given: the checked-in image template and package produced by the bespoke recipe.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "kube-rbac-proxy.yaml"))
	require.NoError(t, err)
	tmpl := def.Types["default"]

	// When: the local Melange artifact is pinned for the image build.
	err = pinLocalPackageVersions(&tmpl, "0", []apkindex.Package{{
		Name:    "kube-rbac-proxy",
		Version: "0.22.1-r3",
	}})

	// Then: apko can only select the approved fixed locally built revision.
	require.NoError(t, err)
	require.Equal(t, []string{"kube-rbac-proxy=0.22.1-r3@local"}, tmpl.Packages)
}
