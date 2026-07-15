package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/integer/apkindex"
	intconfig "github.com/verity-org/verity/internal/integer/config"
)

func Test_ExternalSecrets_uses_pinned_bespoke_package(t *testing.T) {
	// Given: the checked-in External Secrets Operator image definition.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "external-secrets.yaml"))
	require.NoError(t, err)

	// When: the approved default image variant is resolved.
	tmpl := def.Types["default"]

	// Then: the image selects the locally rebuilt package and preserves its runtime contract.
	require.NotNil(t, tmpl.Melange)
	require.Equal(t, "external-secrets-operator.yaml", tmpl.Melange.Bespoke.First())
	require.Equal(t, []string{"external-secrets-operator"}, tmpl.Packages)
	require.Equal(t, "/usr/bin/external-secrets", tmpl.Entrypoint)
}

func Test_ExternalSecrets_recipe_pins_fixed_source_revision(t *testing.T) {
	// Given: the bespoke package recipe selected by the image.
	recipe, err := os.ReadFile(filepath.Join("..", "packages", "bespoke", "external-secrets-operator.yaml"))
	require.NoError(t, err)
	text := string(recipe)

	// When: its package, source, dependency, build, and license metadata are inspected.

	// Then: approved revision r5 rebuilds immutable Apache-2.0 v2.6.0 source with every required fix.
	require.Contains(t, text, "name: external-secrets-operator")
	require.Contains(t, text, "version: \"2.6.0\"")
	require.Contains(t, text, "epoch: 5")
	require.Contains(t, text, "license: Apache-2.0")
	require.Contains(t, text, "Adapted from wolfi-dev/os@253fef5452f6d4c7636f72b697e87eed95dafd94")
	require.Contains(t, text, "cc1ae7fe2927fbe61df9aa87bf2e5075972c79f9.tar.gz")
	require.Contains(t, text, "expected-sha256: c45b054f10c90acf2d5814296205438631c83c17a9bb32d0888703b631d266fb")
	require.Contains(t, text, "go.mongodb.org/mongo-driver@v1.17.7")
	require.Contains(t, text, "go.opentelemetry.io/otel@v1.44.0")
	require.Contains(t, text, "go.opentelemetry.io/otel/sdk@v1.44.0")
	require.Contains(t, text, "go.opentelemetry.io/otel/metric@v1.44.0")
	require.Contains(t, text, "go.opentelemetry.io/otel/trace@v1.44.0")
	require.Contains(t, text, "go-package: go-1.26")
	require.Contains(t, text, "packages: .")
	require.Contains(t, text, "output: external-secrets")
	require.Contains(t, text, "\"${{targets.destdir}}/usr/bin/external-secrets\" --help")
	require.Contains(t, text, "apk info --who-owns /usr/bin/external-secrets")
	require.Contains(t, text, "${{package.name}}-${{package.full-version}}")
	require.Contains(t, text, "usr/share/licenses/${{package.name}}/LICENSE")
}

func Test_ExternalSecrets_resolves_fixed_local_package(t *testing.T) {
	// Given: the checked-in image template and the package produced by the bespoke recipe.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "external-secrets.yaml"))
	require.NoError(t, err)
	tmpl := def.Types["default"]

	// When: local Melange artifacts are pinned for the image build.
	err = pinLocalPackageVersions(&tmpl, "0", []apkindex.Package{{
		Name:    "external-secrets-operator",
		Version: "2.6.0-r5",
	}})

	// Then: apko can only select the approved fixed locally built revision.
	require.NoError(t, err)
	require.Equal(t, []string{"external-secrets-operator=2.6.0-r5@local"}, tmpl.Packages)
}
