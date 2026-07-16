package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/integer/apkindex"
	intconfig "github.com/verity-org/verity/internal/integer/config"
)

func Test_TruffleHog_uses_pinned_bespoke_package(t *testing.T) {
	// Given: the checked-in TruffleHog image definition.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "trufflehog.yaml"))
	require.NoError(t, err)

	// When: the approved default image variant is resolved.
	tmpl := def.Types["default"]

	// Then: the image selects the local rebuild and preserves its runtime contract.
	require.NotNil(t, tmpl.Melange)
	require.Equal(t, "trufflehog.yaml", tmpl.Melange.Bespoke.First())
	require.Equal(t, []string{"trufflehog"}, tmpl.Packages)
	require.Equal(t, "/usr/bin/trufflehog", tmpl.Entrypoint)
}

func Test_TruffleHog_recipe_pins_immutable_fixed_source(t *testing.T) {
	// Given: the bespoke package recipe selected by the image.
	recipe, err := os.ReadFile(filepath.Join("..", "packages", "bespoke", "trufflehog.yaml"))
	require.NoError(t, err)
	text := string(recipe)

	// When: its package, source, dependency, runtime, scan, and license metadata are inspected.

	// Then: revision r1 rebuilds immutable AGPL-3.0 v3.95.9 source with the linked MongoDB fix.
	require.Contains(t, text, "name: trufflehog")
	require.Contains(t, text, "version: \"3.95.9\"")
	require.Contains(t, text, "epoch: 1")
	require.Contains(t, text, "license: AGPL-3.0")
	require.Contains(t, text, "repository: https://github.com/trufflesecurity/trufflehog")
	require.Contains(t, text, "tag: v${{package.version}}")
	require.Contains(t, text, "expected-commit: 27b0417c16317ca9a472a9a8092acce143b49c55")
	require.Contains(t, text, "go.mongodb.org/mongo-driver@v1.17.7")
	require.Contains(t, text, "go-package: go-1.26")
	require.Contains(t, text, "BuildVersion=${{package.version}}")
	require.Contains(t, text, "go version -m")
	require.Contains(t, text, "trufflehog --version")
	require.Contains(t, text, "trufflehog --help")
	require.Contains(t, text, "trufflehog filesystem")
	require.Contains(t, text, "apk info --who-owns /usr/bin/trufflehog")
	require.Contains(t, text, "apk info --license trufflehog")
	require.Contains(t, text, "usr/share/licenses/${{package.name}}/LICENSE")
	require.Contains(t, text, "spdx.json")
	require.Contains(t, text, "licenseDeclared == \"AGPL-3.0\"")
}

func Test_TruffleHog_resolves_fixed_local_package(t *testing.T) {
	// Given: the checked-in image template and package produced by the bespoke recipe.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "trufflehog.yaml"))
	require.NoError(t, err)
	tmpl := def.Types["default"]

	// When: the local Melange artifact is pinned for the image build.
	err = pinLocalPackageVersions(&tmpl, "3", []apkindex.Package{
		{Name: "trufflehog", Version: "3.95.9-r1"},
	})

	// Then: apko can only select the approved fixed locally built revision.
	require.NoError(t, err)
	require.Equal(t, []string{"trufflehog=3.95.9-r1@local"}, tmpl.Packages)
}
