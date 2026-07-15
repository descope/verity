package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/integer/apkindex"
	intconfig "github.com/verity-org/verity/internal/integer/config"
)

func Test_Jaeger_uses_pinned_bespoke_package(t *testing.T) {
	// Given: the checked-in Jaeger image definition.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "jaeger.yaml"))
	require.NoError(t, err)

	// When: the approved default image variant is resolved.
	tmpl := def.Types["default"]

	// Then: the image selects the fixed local all-in-one package and preserves its entrypoint.
	require.NotNil(t, tmpl.Melange)
	require.Equal(t, "jaeger-2-all-in-one.yaml", tmpl.Melange.Bespoke.First())
	require.Equal(t, []string{"jaeger-2-all-in-one"}, tmpl.Packages)
	require.Equal(t, "/usr/bin/all-in-one", tmpl.Entrypoint)
}

func Test_Jaeger_recipe_pins_fixed_source_revision(t *testing.T) {
	// Given: the bespoke package recipe selected by the image.
	recipe, err := os.ReadFile(filepath.Join("..", "packages", "bespoke", "jaeger-2-all-in-one.yaml"))
	require.NoError(t, err)
	text := string(recipe)

	// When: its package, source, dependency, build, test, and license metadata are inspected.

	// Then: approved revision r8 rebuilds immutable Apache-2.0 v2.17.0 source with every required fix.
	require.Contains(t, text, "name: jaeger-2-all-in-one")
	require.Contains(t, text, "version: \"2.17.0\"")
	require.Contains(t, text, "epoch: 8")
	require.Contains(t, text, "license: Apache-2.0")
	require.Contains(t, text, "Adapted from wolfi-dev/os@63a28690a65cf1d91d381afd8769218aab2c59cf")
	require.Contains(t, text, "b5be18b10053a087a9bb272a1d9a0c71bcaac2c7.tar.gz")
	require.Contains(t, text, "expected-sha256: 63d2d9c495dd0151dcd15475e200e08e4bccffb1b3e454a7333bd950c2b5b542")
	require.Contains(t, text, "be495f74ce4fbb68c722dbb8e1cbb6461b052aec.tar.gz")
	require.Contains(t, text, "expected-sha256: 1617359592a041a34844e08acf7e06140c41a1b1236e2cc5952805ae415b6f35")
	require.Contains(t, text, "4e887dcec50fb9f7b675fb4619ea8e3fb4c0447f.tar.gz")
	require.Contains(t, text, "expected-sha256: 1bba3a51884f3f94ab8b79fbbcc719f9aa77bd3baead4fd9c9c919bfa034098e")
	require.Contains(t, text, "go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp@v0.19.0")
	require.Contains(t, text, "go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp@v1.43.0")
	require.Contains(t, text, "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp@v1.43.0")
	require.Contains(t, text, "go.opentelemetry.io/otel/sdk@v1.43.0")
	require.Contains(t, text, "github.com/prometheus/prometheus@v0.311.3")
	require.Contains(t, text, "github.com/apache/thrift@v0.23.0")
	require.Contains(t, text, "go-package: go-1.26")
	require.Contains(t, text, "packages: ./cmd/jaeger")
	require.Contains(t, text, "output: all-in-one")
	require.Contains(t, text, "npm install --prefix jaeger-ui/ --ignore-scripts --no-audit --no-fund")
	require.Contains(t, text, "npm run --workspace @jaegertracing/plexus prepublishOnly")
	require.NotContains(t, text, "npm install --prefix jaeger-ui/\n")
	require.NotContains(t, text, "resolutions.react-icons")
	require.Contains(t, text, "start: env JAEGER_LISTEN_HOST=127.0.0.1 all-in-one")
	require.Contains(t, text, "all-in-one version")
	require.Contains(t, text, "http://127.0.0.1:13133/status")
	require.Contains(t, text, "usr/share/licenses/${{package.name}}/LICENSE")
}

func Test_Jaeger_resolves_fixed_local_package(t *testing.T) {
	// Given: the checked-in image template and the package produced by the bespoke recipe.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "jaeger.yaml"))
	require.NoError(t, err)
	tmpl := def.Types["default"]

	// When: local Melange artifacts are pinned for the image build.
	err = pinLocalPackageVersions(&tmpl, "1", []apkindex.Package{{
		Name:    "jaeger-2-all-in-one",
		Version: "2.17.0-r8",
	}})

	// Then: apko can only select the approved fixed locally built revision.
	require.NoError(t, err)
	require.Equal(t, []string{"jaeger-2-all-in-one=2.17.0-r8@local"}, tmpl.Packages)
}
