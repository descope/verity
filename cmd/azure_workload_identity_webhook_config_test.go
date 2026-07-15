package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/integer/apkindex"
	intconfig "github.com/verity-org/verity/internal/integer/config"
)

func Test_AzureWorkloadIdentityWebhook_uses_pinned_bespoke_package(t *testing.T) {
	// Given: the checked-in Azure Workload Identity Webhook image definition.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "azure-workload-identity-webhook.yaml"))
	require.NoError(t, err)

	// When: the approved default image variant is resolved.
	tmpl := def.Types["default"]

	// Then: the image selects the locally rebuilt package and preserves its runtime contract.
	require.NotNil(t, tmpl.Melange)
	require.Equal(t, "azure-workload-identity-webhook.yaml", tmpl.Melange.Bespoke.First())
	require.Equal(t, []string{"azure-workload-identity-webhook"}, tmpl.Packages)
	require.Equal(t, "/usr/bin/manager", tmpl.Entrypoint)
}

func Test_AzureWorkloadIdentityWebhook_recipe_pins_fixed_source_revision(t *testing.T) {
	// Given: the bespoke package recipe selected by the image.
	recipe, err := os.ReadFile(filepath.Join("..", "packages", "bespoke", "azure-workload-identity-webhook.yaml"))
	require.NoError(t, err)
	text := string(recipe)

	// When: its package, source, version, and license metadata are inspected.

	// Then: revision r4 is rebuilt from immutable MIT-licensed v1.6.0 source.
	require.Contains(t, text, "name: azure-workload-identity-webhook")
	require.Contains(t, text, "version: \"1.6.0\"")
	require.Contains(t, text, "epoch: 4")
	require.Contains(t, text, "license: MIT")
	require.Contains(t, text, "repository: https://github.com/Azure/azure-workload-identity")
	require.Contains(t, text, "tag: v${{package.version}}")
	require.Contains(t, text, "expected-commit: 04d01a2fc7f5024290a38f6ccaf69561cf70455d")
	require.Contains(t, text, "go-package: go-1.26")
	require.Contains(t, text, "packages: ./cmd/webhook")
	require.Contains(t, text, "github.com/Azure/azure-workload-identity/pkg/version.BuildVersion")
	require.Contains(t, text, "github.com/Azure/azure-workload-identity/pkg/version.Vcs")
	require.Contains(t, text, "usr/share/licenses/${{package.name}}/LICENSE")
}

func Test_AzureWorkloadIdentityWebhook_resolves_fixed_local_package(t *testing.T) {
	// Given: the checked-in image template and the package produced by the bespoke recipe.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "azure-workload-identity-webhook.yaml"))
	require.NoError(t, err)
	tmpl := def.Types["default"]

	// When: local Melange artifacts are pinned for the image build.
	err = pinLocalPackageVersions(&tmpl, "1", []apkindex.Package{{
		Name:    "azure-workload-identity-webhook",
		Version: "1.6.0-r4",
	}})

	// Then: apko can only select the approved fixed locally built revision.
	require.NoError(t, err)
	require.Equal(t, []string{"azure-workload-identity-webhook=1.6.0-r4@local"}, tmpl.Packages)
}
