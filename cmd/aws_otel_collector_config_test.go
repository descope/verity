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

	// Then: revision r7 is rebuilt from immutable Apache-2.0 source with both approved fixes.
	require.Contains(t, text, "name: aws-otel-collector")
	require.Contains(t, text, "version: \"0.48.0\"")
	require.Contains(t, text, "epoch: 7")
	require.Contains(t, text, "license: Apache-2.0")
	require.Contains(t, text, "repository: https://github.com/aws-observability/aws-otel-collector")
	require.Contains(t, text, "tag: v${{package.version}}")
	require.Contains(t, text, "expected-commit: 4454fb1af28b47947f59e522d5ed874d27dcc621")
	require.Contains(t, text, "uses: go/bump")
	require.NotContains(t, text, "uses: bump")
	require.Contains(t, text, "modroot: .")
	require.NotContains(t, text, "modroot: |-")
	require.Contains(t, text, "go.opentelemetry.io/otel/sdk@v1.44.0")
	require.Contains(t, text, "go.opentelemetry.io/otel@v1.44.0")
	require.Contains(t, text, "go.opentelemetry.io/otel/trace@v1.44.0")
	require.Contains(t, text, "go.opentelemetry.io/otel/metric@v1.44.0")
	require.Contains(t, text, "go-package: go-1.26")
	require.Contains(t, text, "bins: awscollector")
}

func Test_AWSOTelCollector_resolves_fixed_local_package(t *testing.T) {
	// Given: the checked-in image template and the package produced by the bespoke recipe.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "aws-otel-collector.yaml"))
	require.NoError(t, err)
	tmpl := def.Types["default"]

	// When: local Melange artifacts are pinned for the image build.
	err = pinLocalPackageVersions(&tmpl, "0", []apkindex.Package{{
		Name:    "aws-otel-collector",
		Version: "0.48.0-r7",
	}})

	// Then: apko can only select the fixed locally built revision.
	require.NoError(t, err)
	require.Equal(t, []string{"aws-otel-collector=0.48.0-r7@local"}, tmpl.Packages)
}
