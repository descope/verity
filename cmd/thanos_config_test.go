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

func Test_Thanos_uses_fixed_bespoke_package_for_every_variant(t *testing.T) {
	// Given: the checked-in Thanos image definition.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "thanos.yaml"))
	require.NoError(t, err)

	// When: every supported image variant is resolved.
	for _, variant := range []string{"default", "fips"} {
		t.Run(variant, func(t *testing.T) {
			tmpl := def.Types[variant]

			// Then: each variant selects the same local rebuild and runnable binary.
			require.NotNil(t, tmpl.Melange)
			require.Equal(t, "thanos.yaml", tmpl.Melange.Bespoke.First())
			require.Equal(t, []string{"thanos"}, tmpl.Packages)
			require.Equal(t, "/usr/bin/thanos", tmpl.Entrypoint)
		})
	}
	require.Empty(t, def.Versions["0"].SkipTypes)
}

func Test_Thanos_recipe_pins_immutable_fixed_source(t *testing.T) {
	// Given: the bespoke package recipe selected by the image family.
	recipe, err := os.ReadFile(filepath.Join("..", "packages", "bespoke", "thanos.yaml"))
	require.NoError(t, err)
	text := string(recipe)

	// When: its package, source, dependency, test, and license metadata are inspected.

	// Then: revision r18 rebuilds immutable Apache-2.0 v0.41.0 source with both current fixes.
	require.Contains(t, text, "name: thanos")
	require.Contains(t, text, "# renovate: datasource=github-tags depName=thanos-io/thanos versioning=semver-coerced")
	require.Contains(t, text, "version: \"0.41.0\"")
	require.Contains(t, text, "epoch: 18")
	require.Contains(t, text, "license: Apache-2.0")
	require.Contains(t, text, "Adapted from wolfi-dev/os@17a5c17c4c614f1e38d5b5199ba5e7810f0d3151")
	require.Contains(t, text, "cb1396b916241f63fff75e4f5362ea65f18f2303.tar.gz")
	require.Contains(t, text, "expected-sha256: 90ec49ab04298c9f4bfa2a08771f8d02cad6ef638baf74a1a00c5950151b53e1")
	require.Contains(t, text, "go get \\")
	require.Contains(t, text, "go.mongodb.org/mongo-driver@v1.17.7")
	require.Contains(t, text, "go.opentelemetry.io/otel@v1.44.0")
	require.Contains(t, text, "go.opentelemetry.io/otel/sdk@v1.44.0")
	require.Contains(t, text, "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp@v1.44.0")
	require.Contains(t, text, "golang.org/x/text@v0.37.0")
	require.Contains(t, text, "go list -m -f '{{.Version}}' go.opentelemetry.io/otel")
	require.Contains(t, text, "go-package: go-1.26")
	require.Contains(t, text, "usr/share/licenses/${{package.name}}/LICENSE")
	require.Contains(t, text, "thanos tools bucket verify --objstore.config-file=/tmp/filesystem.yaml")
	require.Contains(t, text, "thanos query --http-address=127.0.0.1:19192")
	require.Contains(t, text, "curl --fail-with-body --silent --show-error --retry 5")
	require.NotContains(t, text, "iamguarded")
	require.Equal(t, 1, strings.Count(text, "apk info --license thanos"))
}

func Test_Thanos_resolves_fixed_local_package_for_every_variant(t *testing.T) {
	// Given: the checked-in image templates and the package produced by the bespoke recipe.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "thanos.yaml"))
	require.NoError(t, err)

	for _, variant := range []string{"default", "fips"} {
		t.Run(variant, func(t *testing.T) {
			tmpl := def.Types[variant]

			// When: the local Melange artifact is pinned for the image build.
			pinErr := pinLocalPackageVersions(&tmpl, "0", []apkindex.Package{{
				Name:    "thanos",
				Version: "0.41.0-r18",
			}})

			// Then: apko can only select the approved fixed locally built revision.
			require.NoError(t, pinErr)
			require.Equal(t, []string{"thanos=0.41.0-r18@local"}, tmpl.Packages)
		})
	}
}
