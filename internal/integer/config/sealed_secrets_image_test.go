package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_SealedSecrets_uses_owned_family_package_when_default_image_is_resolved(t *testing.T) {
	// Given: the checked-in Sealed Secrets image definition.
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok, "locating test file")
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
	def, err := LoadImage(filepath.Join(repoRoot, "images", "sealed-secrets.yaml"))
	require.NoError(t, err)
	require.NoError(t, Validate(def))

	// When: the default controller image variant is resolved.
	tmpl := def.Types["default"]

	// Then: the existing v0 family tag selects only the immutable owned package.
	require.NotNil(t, tmpl.Melange)
	require.Equal(t, "sealed-secrets-0.yaml", tmpl.Melange.Bespoke.First())
	require.Equal(t, []string{"sealed-secrets-0"}, tmpl.Packages)
	require.Equal(t, "/usr/bin/controller", tmpl.Entrypoint)
	require.Contains(t, def.Versions, "0")
}

func Test_SealedSecrets_recipe_pins_controller_kubeseal_and_spdx_metadata(t *testing.T) {
	// Given: the bespoke package recipe selected by the Sealed Secrets image.
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok, "locating test file")
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
	recipe, err := os.ReadFile(filepath.Join(repoRoot, "packages", "bespoke", "sealed-secrets-0.yaml"))
	require.NoError(t, err)
	text := string(recipe)

	// When: source, runtime, both binaries, and SPDX-facing license metadata are inspected.

	// Then: the recipe is immutable, carries its CA runtime, and packages both supported executables.
	require.Contains(t, text, "name: sealed-secrets-0")
	require.Contains(t, text, "version: \"0.38.4\"")
	require.Contains(t, text, "license: Apache-2.0")
	require.Contains(t, text, "repository: https://github.com/bitnami/sealed-secrets")
	require.Contains(t, text, "tag: v${{package.version}}")
	require.Contains(t, text, "expected-commit: 0cc547ad0c3756726768dcc916e39de3ac5b101b")
	require.Contains(t, text, "runtime:\n      - ca-certificates-bundle")
	require.Contains(t, text, "packages: ./cmd/controller")
	require.Contains(t, text, "output: controller")
	require.Contains(t, text, "packages: ./cmd/kubeseal")
	require.Contains(t, text, "output: kubeseal")
	require.Contains(t, text, "name: ${{package.name}}-kubeseal")
	require.Contains(t, text, "provides:\n        - kubeseal=${{package.full-version}}")
	require.Contains(t, text, "install -Dm644 LICENSE")
	require.Contains(t, text, "controller --version")
	require.Contains(t, text, "kubeseal --help")
	require.Contains(t, text, "apk info --provides \"${{package.name}}-kubeseal\"")
}
