package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/integer/apkindex"
	intconfig "github.com/verity-org/verity/internal/integer/config"
)

func Test_Hydra_uses_pinned_bespoke_package(t *testing.T) {
	// Given: the checked-in Ory Hydra image definition.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "hydra.yaml"))
	require.NoError(t, err)

	// When: the approved default image variant is resolved.
	tmpl := def.Types["default"]

	// Then: the image selects the locally rebuilt package and preserves its runtime contract.
	require.NotNil(t, tmpl.Melange)
	require.Equal(t, "hydra.yaml", tmpl.Melange.Bespoke.First())
	require.Equal(t, []string{"hydra"}, tmpl.Packages)
	require.Equal(t, "/usr/bin/hydra", tmpl.Entrypoint)
}

func Test_Hydra_recipe_pins_fixed_source_revision(t *testing.T) {
	// Given: the bespoke package recipe selected by the image.
	recipe, err := os.ReadFile(filepath.Join("..", "packages", "bespoke", "hydra.yaml"))
	require.NoError(t, err)
	text := string(recipe)

	// When: its package, source, dependency, build, and license metadata are inspected.

	// Then: approved revision r16 rebuilds immutable Apache-2.0 v26.2.0 source with both required fixes.
	require.Contains(t, text, "name: hydra")
	require.Contains(t, text, "version: \"26.2.0\"")
	require.Contains(t, text, "epoch: 16")
	require.Contains(t, text, "license: Apache-2.0")
	require.Contains(t, text, "Adapted from wolfi-dev/os@0c73a9cad82af43a57378c3a0da356f9a8c4a740")
	require.Contains(t, text, "0b84568fffccf151dc5e6c7955fdfb738555bf4b.tar.gz")
	require.Contains(t, text, "expected-sha256: 7ceaae3299780959e8390925732629931f63f20300464d2822d49628eeb3332e")
	require.Contains(t, text, "go.opentelemetry.io/otel@v1.44.0")
	require.Contains(t, text, "go.opentelemetry.io/otel/sdk@v1.44.0")
	require.Contains(t, text, "go.opentelemetry.io/otel/trace@v1.44.0")
	require.Contains(t, text, "go-package: go-1.26")
	require.Contains(t, text, "packages: .")
	require.Contains(t, text, "output: hydra")
	require.Contains(t, text, "tags: sqlite,json1,hsm")
	require.Contains(t, text, "/usr/bin/hydra version")
	require.Contains(t, text, "apk info --who-owns /usr/bin/hydra")
	require.Contains(t, text, "usr/share/licenses/${{package.name}}/LICENSE")
}

func Test_Hydra_resolves_fixed_local_package(t *testing.T) {
	// Given: the checked-in image template and the package produced by the bespoke recipe.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "hydra.yaml"))
	require.NoError(t, err)
	tmpl := def.Types["default"]

	// When: local Melange artifacts are pinned for the image build.
	err = pinLocalPackageVersions(&tmpl, "2", []apkindex.Package{{
		Name:    "hydra",
		Version: "26.2.0-r16",
	}})

	// Then: apko can only select the approved fixed locally built revision.
	require.NoError(t, err)
	require.Equal(t, []string{"hydra=26.2.0-r16@local"}, tmpl.Packages)
}
