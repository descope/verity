package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_Tempo_uses_owned_2_10_package(t *testing.T) {
	// Given: the checked-in Tempo image definition.
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok, "locating test file")
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
	def, err := LoadImage(filepath.Join(repoRoot, "images", "tempo.yaml"))
	require.NoError(t, err)
	require.NoError(t, Validate(def))

	// When: the approved default image variant is resolved.
	tmpl := def.Types["default"]

	// Then: both published aliases use the family-owned 2.10 package without changing runtime semantics.
	require.NotNil(t, tmpl.Melange)
	require.Equal(t, "tempo-2.10.yaml", tmpl.Melange.Bespoke.First())
	require.Equal(t, []string{"tempo-2.10"}, tmpl.Packages)
	require.Equal(t, "/usr/bin/tempo", tmpl.Entrypoint)
	require.Contains(t, def.Versions, "2")
	require.Contains(t, def.Versions, "2.10.0")
	require.NotContains(t, def.Versions, "2.9.0")
}

func Test_Tempo_recipe_pins_immutable_security_release(t *testing.T) {
	// Given: the bespoke package recipe selected by the Tempo image.
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok, "locating test file")
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
	recipe, err := os.ReadFile(filepath.Join(repoRoot, "packages", "bespoke", "tempo-2.10.yaml"))
	require.NoError(t, err)
	text := string(recipe)

	// When: its package, source, build, test, and license metadata are inspected.

	// Then: revision r0 rebuilds immutable v2.10.7 source with its declared project licenses and security fixes.
	require.Contains(t, text, "name: tempo-2.10")
	require.Contains(t, text, "version: \"2.10.7\"")
	require.Contains(t, text, "epoch: 3")
	require.Contains(t, text, "license: AGPL-3.0-only AND Apache-2.0")
	require.Contains(t, text, "Adapted from wolfi-dev/os@1e46ec097ff71bd5eccd13fa97a014f530eeec42")
	require.Contains(t, text, "8100f5a7b8f0986edf1fcd2ff6552d174b25ec18.tar.gz")
	require.Contains(t, text, "expected-sha256: 9ad4c9c6e73b67ae32e00f282dc14d81f1b1eb9b49607c58803e3d663fe1e527")
	require.Contains(t, text, "runtime:\n      - ca-certificates-bundle")
	require.Contains(t, text, "go-package: go-1.26")
	require.Contains(t, text, "uses: go/bump")
	require.Contains(t, text, "go.opentelemetry.io/otel@v1.44.0")
	require.Contains(t, text, "github.com/prometheus/prometheus v0.311.3")
	require.Contains(t, text, "packages: ./cmd/tempo")
	require.Contains(t, text, "output: tempo")
	require.Contains(t, text, "-X main.Version=${{package.version}}")
	require.Contains(t, text, "-X main.Revision=8100f5a7b8f0986edf1fcd2ff6552d174b25ec18")
	require.Contains(t, text, "-X main.Branch=v${{package.version}}")
	require.Contains(t, text, "tempo --version")
	require.Contains(t, text, "http_listen_port: 3200")
	require.Contains(t, text, "seq 1 200")
	require.Contains(t, text, "http://127.0.0.1:3200/ready")
	require.Contains(t, text, "/usr/share/licenses/${{package.name}}/LICENSING.md")
	require.Contains(t, text, "/usr/share/licenses/${{package.name}}/LICENSE_APACHE2")
}
