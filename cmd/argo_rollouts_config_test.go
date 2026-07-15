package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	intconfig "github.com/verity-org/verity/internal/integer/config"
)

func Test_ArgoRollouts_uses_pinned_bespoke_package(t *testing.T) {
	// Given: the checked-in Argo Rollouts image definition.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "argo-rollouts.yaml"))
	require.NoError(t, err)

	// When: the default image variant is resolved.
	tmpl := def.Types["default"]

	// Then: the image selects the locally rebuilt package and preserves its runtime contract.
	require.NotNil(t, tmpl.Melange)
	require.Equal(t, "argo-rollouts.yaml", tmpl.Melange.Bespoke.First())
	require.Equal(t, []string{"argo-rollouts"}, tmpl.Packages)
	require.Equal(t, "/usr/bin/rollouts-controller", tmpl.Entrypoint)
}

func Test_ArgoRollouts_recipe_pins_fixed_source_revision(t *testing.T) {
	// Given: the bespoke package recipe selected by the image.
	recipe, err := os.ReadFile(filepath.Join("..", "packages", "bespoke", "argo-rollouts.yaml"))
	require.NoError(t, err)
	text := string(recipe)

	// When: its package and source metadata are inspected.

	// Then: revision r19 contains substantive dependency fixes from immutable Apache-2.0 source.
	require.Contains(t, text, "name: argo-rollouts")
	require.Contains(t, text, "version: \"1.9.0\"")
	require.Contains(t, text, "epoch: 19")
	require.Contains(t, text, "license: Apache-2.0")
	require.Contains(t, text, "archive/838d4e792be666ec11bd0c80331e0c5511b5010e.tar.gz")
	require.Contains(t, text, "expected-sha256: 9f0ab3e26904a1c02eb7051441b8ddba89e9cf342c297573edfe33e7d1f03603")
	require.Contains(t, text, "go-1.26=1.26.5-r1")
	require.Contains(t, text, "go.opentelemetry.io/otel@v1.44.0")
	require.Contains(t, text, "go.opentelemetry.io/otel/trace@v1.44.0")
}
