package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/integer/apkindex"
	intconfig "github.com/verity-org/verity/internal/integer/config"
)

func Test_AWSNodeTerminationHandler_uses_pinned_bespoke_package(t *testing.T) {
	// Given: the checked-in AWS Node Termination Handler image definition.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "aws-node-termination-handler.yaml"))
	require.NoError(t, err)

	// When: the approved default image variant is resolved.
	tmpl := def.Types["default"]

	// Then: the image selects the locally rebuilt package and preserves its runtime contract.
	require.NotNil(t, tmpl.Melange)
	require.Equal(t, "aws-node-termination-handler.yaml", tmpl.Melange.Bespoke.First())
	require.Equal(t, []string{"aws-node-termination-handler"}, tmpl.Packages)
	require.Equal(t, "/usr/bin/node-termination-handler", tmpl.Entrypoint)
}

func Test_AWSNodeTerminationHandler_recipe_pins_fixed_source_revision(t *testing.T) {
	// Given: the bespoke package recipe selected by the image.
	recipe, err := os.ReadFile(filepath.Join("..", "packages", "bespoke", "aws-node-termination-handler.yaml"))
	require.NoError(t, err)
	text := string(recipe)

	// When: its package, source, and dependency metadata are inspected.

	// Then: revision r13 is rebuilt from immutable Apache-2.0 source with both approved fixes.
	require.Contains(t, text, "name: aws-node-termination-handler")
	require.Contains(t, text, "version: \"1.25.6\"")
	require.Contains(t, text, "epoch: 13")
	require.Contains(t, text, "license: Apache-2.0")
	require.Contains(t, text, "repository: https://github.com/aws/aws-node-termination-handler")
	require.Contains(t, text, "tag: v${{package.version}}")
	require.Contains(t, text, "expected-commit: 77ca841d1329504e981542775f78403feb114048")
	require.Contains(t, text, "go.opentelemetry.io/otel/sdk@v1.44.0")
	require.Contains(t, text, "go.opentelemetry.io/otel@v1.44.0")
	require.Contains(t, text, "go.opentelemetry.io/otel/trace@v1.44.0")
	require.Contains(t, text, "go.opentelemetry.io/otel/metric@v1.44.0")
	require.Contains(t, text, "go-package: go-1.26")
	require.Contains(t, text, "node-termination-handler --help")
}

func Test_AWSNodeTerminationHandler_resolves_fixed_local_package(t *testing.T) {
	// Given: the checked-in image template and the package produced by the bespoke recipe.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "aws-node-termination-handler.yaml"))
	require.NoError(t, err)
	tmpl := def.Types["default"]

	// When: local Melange artifacts are pinned for the image build.
	err = pinLocalPackageVersions(&tmpl, "1", []apkindex.Package{{
		Name:    "aws-node-termination-handler",
		Version: "1.25.6-r13",
	}})

	// Then: apko can only select the fixed locally built revision.
	require.NoError(t, err)
	require.Equal(t, []string{"aws-node-termination-handler=1.25.6-r13@local"}, tmpl.Packages)
}
