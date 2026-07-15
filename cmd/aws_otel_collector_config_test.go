package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/integer/apkindex"
	intconfig "github.com/verity-org/verity/internal/integer/config"
)

func Test_AWSOTelCollector_uses_pinned_bespoke_package(t *testing.T) {
	// Given: the checked-in AWS OTel Collector image definition.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "aws-otel-collector.yaml"))
	require.NoError(t, err)

	// When: the approved default image variant is resolved.
	tmpl := def.Types["default"]

	// Then: the image selects the locally rebuilt package and preserves its runtime contract.
	require.NotNil(t, tmpl.Melange)
	require.Equal(t, "aws-otel-collector.yaml", tmpl.Melange.Bespoke.First())
	require.Equal(t, []string{"aws-otel-collector"}, tmpl.Packages)
	require.Equal(t, "/usr/bin/awscollector", tmpl.Entrypoint)
}

func Test_AWSOTelCollector_recipe_pins_fixed_source_revision(t *testing.T) {
	// Given: the bespoke package recipe selected by the image.
	recipe, err := os.ReadFile(filepath.Join("..", "packages", "bespoke", "aws-otel-collector.yaml"))
	require.NoError(t, err)
	text := string(recipe)

	// When: its package, source, dependency, and license metadata are inspected.

	// Then: the fixed r0 release is rebuilt from immutable Apache-2.0 source.
	require.Contains(t, text, "name: aws-otel-collector")
	require.Contains(t, text, "version: \"0.49.0\"")
	require.Contains(t, text, "epoch: 0")
	require.Contains(t, text, "license: Apache-2.0")
	require.Contains(t, text, "repository: https://github.com/aws-observability/aws-otel-collector")
	require.Contains(t, text, "tag: v${{package.version}}")
	require.Contains(t, text, "expected-commit: 0771477f9db2879afad3ae3ff7811b5264a151a8")
	require.Contains(t, text, "go-package: go-1.26")
	require.Contains(t, text, "bins: awscollector")
	require.Contains(t, text, `  - name: ${{package.name}}-compat
    dependencies:
      runtime:
        - ${{package.name}}
        - ${{package.name}}-healthcheck
`)
}

func Test_AWSOTelCollector_resolves_fixed_local_package(t *testing.T) {
	// Given: the checked-in image template and the package produced by the bespoke recipe.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "aws-otel-collector.yaml"))
	require.NoError(t, err)
	tmpl := def.Types["default"]

	// When: local Melange artifacts are pinned for the image build.
	err = pinLocalPackageVersions(&tmpl, "0", []apkindex.Package{{
		Name:    "aws-otel-collector",
		Version: "0.49.0-r0",
	}})

	// Then: apko can only select the fixed locally built revision.
	require.NoError(t, err)
	require.Equal(t, []string{"aws-otel-collector=0.49.0-r0@local"}, tmpl.Packages)
}
