package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/integer/apkindex"
	intconfig "github.com/verity-org/verity/internal/integer/config"
)

func Test_TFLint_uses_pinned_bespoke_package(t *testing.T) {
	// Given: the checked-in TFLint image definition.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "tflint.yaml"))
	require.NoError(t, err)

	// When: the approved default image variant is resolved.
	tmpl := def.Types["default"]

	// Then: the image selects the local rebuild and preserves its runtime contract.
	require.NotNil(t, tmpl.Melange)
	require.Equal(t, "tflint.yaml", tmpl.Melange.Bespoke.First())
	require.Equal(t, []string{"tflint"}, tmpl.Packages)
	require.Equal(t, "/usr/bin/tflint", tmpl.Entrypoint)
}

func Test_TFLint_recipe_pins_immutable_fixed_source(t *testing.T) {
	// Given: the bespoke package recipe selected by the image.
	recipe, err := os.ReadFile(filepath.Join("..", "packages", "bespoke", "tflint.yaml"))
	require.NoError(t, err)
	text := string(recipe)

	// When: its package, source, build, test, and license metadata are inspected.

	// Then: approved revision r6 rebuilds immutable MPL-2.0 v0.63.1 source with fixed dependencies and Go.
	require.Contains(t, text, "name: tflint")
	require.Contains(t, text, "version: \"0.63.1\"")
	require.Contains(t, text, "epoch: 6")
	require.Contains(t, text, "license: MPL-2.0")
	require.Contains(t, text, "cd0cce4fa3decaabba3c0667c235651ac06a4221.tar.gz")
	require.Contains(t, text, "expected-sha256: 8d9b5aeba7b82640fa21f80d2f490180ed72232f0158cd1e04e91260a41be1a9")
	require.Contains(t, text, "github.com/sigstore/sigstore-go@v1.2.0")
	require.Contains(t, text, "github.com/sigstore/timestamp-authority/v2@v2.1.2")
	require.Contains(t, text, "go.opentelemetry.io/otel@v1.44.0")
	require.Contains(t, text, "go.opentelemetry.io/otel/metric@v1.44.0")
	require.Contains(t, text, "go.opentelemetry.io/otel/sdk@v1.44.0")
	require.Contains(t, text, "go.opentelemetry.io/otel/trace@v1.44.0")
	require.Contains(t, text, "go-package: go-1.26")
	require.Contains(t, text, "packages: .")
	require.Contains(t, text, "output: tflint")
	require.Contains(t, text, "TFLINT_DISABLE_VERSION_CHECK=1")
	require.Contains(t, text, "usr/share/licenses/${{package.name}}/LICENSE")
	require.Contains(t, text, "tflint --version")
	require.Contains(t, text, "apk info --who-owns /usr/bin/tflint")
}

func Test_TFLint_resolves_fixed_local_package(t *testing.T) {
	// Given: the checked-in image template and package produced by the bespoke recipe.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "tflint.yaml"))
	require.NoError(t, err)
	tmpl := def.Types["default"]

	// When: the local Melange artifact is pinned for the image build.
	err = pinLocalPackageVersions(&tmpl, "0", []apkindex.Package{
		{Name: "tflint", Version: "0.63.1-r6"},
	})

	// Then: apko can only select the approved fixed locally built revision.
	require.NoError(t, err)
	require.Equal(t, []string{"tflint=0.63.1-r6@local"}, tmpl.Packages)
}
