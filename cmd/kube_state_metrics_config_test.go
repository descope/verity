package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/integer/apkindex"
	intconfig "github.com/verity-org/verity/internal/integer/config"
)

func Test_KubeStateMetrics_uses_pinned_bespoke_package(t *testing.T) {
	// Given: the checked-in kube-state-metrics image definition.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "kube-state-metrics.yaml"))
	require.NoError(t, err)

	// When: the approved default image variant is resolved.
	tmpl := def.Types["default"]

	// Then: the image selects the local rebuild and preserves its version and runtime contracts.
	require.NotNil(t, tmpl.Melange)
	require.Equal(t, "kube-state-metrics.yaml", tmpl.Melange.Bespoke.First())
	require.Equal(t, []string{"kube-state-metrics"}, tmpl.Packages)
	require.Equal(t, "/usr/bin/kube-state-metrics", tmpl.Entrypoint)
	require.Contains(t, def.Versions, "2.18")
}

func Test_KubeStateMetrics_recipe_pins_fixed_source_revision(t *testing.T) {
	// Given: the bespoke package recipe selected by the image.
	recipe, err := os.ReadFile(filepath.Join("..", "packages", "bespoke", "kube-state-metrics.yaml"))
	require.NoError(t, err)
	text := string(recipe)

	// When: its package, source, build, test, and license metadata are inspected.

	// Then: approved revision r2 rebuilds immutable Apache-2.0 v2.19.1 source with the fixed toolchain.
	require.Contains(t, text, "name: kube-state-metrics")
	require.Contains(t, text, "# renovate: datasource=github-tags depName=kubernetes/kube-state-metrics versioning=semver-coerced")
	require.Contains(t, text, "version: \"2.19.1\"")
	require.Contains(t, text, "epoch: 2")
	require.Contains(t, text, "license: Apache-2.0")
	require.Contains(t, text, "Adapted from wolfi-dev/os@3c2a196ecb89848f52929d91659461a5edd8764c")
	require.Contains(t, text, "45e1513e95beaaa2f2df92af9958cd9837bb66ab.tar.gz")
	require.Contains(t, text, "expected-sha256: cbe6345fe3b927d0c8f7732eca5d14af300f928511d09548181d437210f99ca7")
	require.Contains(t, text, "go-package: go-1.26")
	require.Contains(t, text, "packages: .")
	require.Contains(t, text, "output: kube-state-metrics")
	require.Contains(t, text, "github.com/prometheus/common/version.Version=${{package.version}}")
	require.Contains(t, text, "github.com/prometheus/common/version.BuildDate=2026-06-11T18:22:59Z")
	require.Contains(t, text, "usr/share/licenses/${{package.name}}/LICENSE")
	require.Contains(t, text, "test:\n  environment:\n    contents:\n      packages:\n        - apk-tools\n        - busybox")
	require.Contains(t, text, "kube-state-metrics version")
	require.Contains(t, text, "apk info --who-owns /usr/bin/kube-state-metrics")
}

func Test_KubeStateMetrics_resolves_fixed_local_package(t *testing.T) {
	// Given: the checked-in image template and the package produced by the bespoke recipe.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "kube-state-metrics.yaml"))
	require.NoError(t, err)
	tmpl := def.Types["default"]

	// When: the local Melange artifact is pinned for the image build.
	err = pinLocalPackageVersions(&tmpl, "2.18", []apkindex.Package{{
		Name:    "kube-state-metrics",
		Version: "2.19.1-r2",
	}})

	// Then: apko can only select the approved fixed locally built revision.
	require.NoError(t, err)
	require.Equal(t, []string{"kube-state-metrics=2.19.1-r2@local"}, tmpl.Packages)
}
