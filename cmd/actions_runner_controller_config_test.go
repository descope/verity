package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/integer/apkindex"
	intconfig "github.com/verity-org/verity/internal/integer/config"
)

func Test_ActionsRunnerController_uses_pinned_bespoke_package(t *testing.T) {
	// Given: the checked-in actions-runner-controller image definition.
	repo := ".."
	def, err := intconfig.LoadImage(filepath.Join(repo, "images", "actions-runner-controller.yaml"))
	require.NoError(t, err)

	// When: the default image variant is resolved.
	tmpl := def.Types["default"]

	// Then: the image selects the locally rebuilt package and preserves its runtime contract.
	require.NotNil(t, tmpl.Melange)
	require.Equal(t, "actions-runner-controller.yaml", tmpl.Melange.Bespoke.First())
	require.Equal(t, []string{"actions-runner-controller"}, tmpl.Packages)
	require.Equal(t, "/usr/bin/manager", tmpl.Entrypoint)
}

func Test_ActionsRunnerController_recipe_pins_fixed_source_revision(t *testing.T) {
	// Given: the bespoke package recipe selected by the image.
	recipe, err := os.ReadFile(filepath.Join("..", "packages", "bespoke", "actions-runner-controller.yaml"))
	require.NoError(t, err)
	text := string(recipe)

	// When: its package and source metadata are inspected.

	// Then: the fixed revision is substantive and the upstream source is immutable.
	require.Contains(t, text, "name: actions-runner-controller")
	require.Contains(t, text, "version: \"0.14.2\"")
	require.Contains(t, text, "epoch: 5")
	require.Contains(t, text, "license: Apache-2.0")
	require.Contains(t, text, "repository: https://github.com/actions/actions-runner-controller")
	require.Contains(t, text, "tag: gha-runner-scale-set-${{package.version}}")
	require.Contains(t, text, "expected-commit: 9bb16ae49d0ce585d8e682aa7e2668a6e832d5d8")
	require.Contains(t, text, "packages: ./cmd/ghalistener")
	require.Contains(t, text, "packages: ./cmd/githubwebhookserver")
	require.Contains(t, text, "packages: ./cmd/actionsmetricsserver")
}

func Test_ActionsRunnerController_recipe_tests_ghalistener_configuration_rejection(t *testing.T) {
	// Given: the bespoke package recipe selected by the image.
	recipe, err := os.ReadFile(filepath.Join("..", "packages", "bespoke", "actions-runner-controller.yaml"))
	require.NoError(t, err)
	text := string(recipe)

	// When: the listener smoke-test contract is inspected.

	// Then: the non-CLI binary is exercised offline through its required configuration boundary.
	require.Contains(t, text, `ghalistener 2>&1 | grep -F "LISTENER_CONFIG_PATH environment variable is not set"`)
	require.NotContains(t, text, "ghalistener --help")
}

func Test_ActionsRunnerController_resolves_fixed_local_package(t *testing.T) {
	// Given: the checked-in image template and the package produced by the bespoke recipe.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "actions-runner-controller.yaml"))
	require.NoError(t, err)
	tmpl := def.Types["default"]

	// When: local Melange artifacts are pinned for the image build.
	err = pinLocalPackageVersions(&tmpl, "0", []apkindex.Package{{
		Name:    "actions-runner-controller",
		Version: "0.14.2-r5",
	}})

	// Then: apko can only select the fixed locally built revision.
	require.NoError(t, err)
	require.Equal(t, []string{"actions-runner-controller=0.14.2-r5@local"}, tmpl.Packages)
}
