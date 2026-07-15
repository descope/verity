package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/integer/apkindex"
	intconfig "github.com/verity-org/verity/internal/integer/config"
)

func Test_APISIXIngressController_uses_pinned_bespoke_package(t *testing.T) {
	// Given: the checked-in APISIX Ingress Controller image definition.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "apisix-ingress-controller.yaml"))
	require.NoError(t, err)

	// When: the default image variant is resolved.
	tmpl := def.Types["default"]

	// Then: the image selects the locally rebuilt package and preserves its runtime contract.
	require.NotNil(t, tmpl.Melange)
	require.Equal(t, "apisix-ingress-controller.yaml", tmpl.Melange.Bespoke.First())
	require.Equal(t, []string{"apisix-ingress-controller"}, tmpl.Packages)
	require.Equal(t, "/usr/bin/apisix-ingress-controller", tmpl.Entrypoint)
}

func Test_APISIXIngressController_recipe_pins_fixed_source_revision(t *testing.T) {
	// Given: the bespoke package recipe selected by the image.
	recipe, err := os.ReadFile(filepath.Join("..", "packages", "bespoke", "apisix-ingress-controller.yaml"))
	require.NoError(t, err)
	text := string(recipe)

	// When: its package, source, and dependency metadata are inspected.

	// Then: revision r5 is substantive and the upstream source is immutable.
	require.Contains(t, text, "name: apisix-ingress-controller")
	require.Contains(t, text, "version: \"2.1.0\"")
	require.Contains(t, text, "epoch: 5")
	require.Contains(t, text, "license: Apache-2.0")
	require.Contains(t, text, "repository: https://github.com/apache/apisix-ingress-controller")
	require.Contains(t, text, "tag: ${{package.version}}")
	require.Contains(t, text, "expected-commit: f4a8dd1223573a5b72e1c7b65b37c13b49d98042")
	require.Contains(t, text, "go.opentelemetry.io/otel@v1.44.0")
	require.Contains(t, text, "go.opentelemetry.io/otel/sdk@v1.44.0")
	require.Contains(t, text, "go.opentelemetry.io/otel/metric@v1.44.0")
	require.Contains(t, text, "go.opentelemetry.io/otel/trace@v1.44.0")
}

func Test_APISIXIngressController_resolves_fixed_local_package(t *testing.T) {
	// Given: the checked-in image template and the package produced by the bespoke recipe.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "apisix-ingress-controller.yaml"))
	require.NoError(t, err)
	tmpl := def.Types["default"]

	// When: local Melange artifacts are pinned for the image build.
	err = pinLocalPackageVersions(&tmpl, "1", []apkindex.Package{{
		Name:    "apisix-ingress-controller",
		Version: "2.1.0-r5",
	}})

	// Then: apko can only select the fixed locally built revision.
	require.NoError(t, err)
	require.Equal(t, []string{"apisix-ingress-controller=2.1.0-r5@local"}, tmpl.Packages)
}
