package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/integer/apkindex"
	intconfig "github.com/verity-org/verity/internal/integer/config"
)

func Test_Trivy_uses_pinned_bespoke_package(t *testing.T) {
	// Given: the checked-in Trivy image definition.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "trivy.yaml"))
	require.NoError(t, err)

	// When: the approved default image variant is resolved.
	tmpl := def.Types["default"]

	// Then: the image selects the local rebuild and preserves its runtime contract.
	require.NotNil(t, tmpl.Melange)
	require.Equal(t, "trivy.yaml", tmpl.Melange.Bespoke.First())
	require.Equal(t, []string{"trivy"}, tmpl.Packages)
	require.Equal(t, "/usr/bin/trivy", tmpl.Entrypoint)
}

func Test_Trivy_recipe_pins_immutable_clean_source(t *testing.T) {
	// Given: the bespoke package recipe selected by the image.
	recipe, err := os.ReadFile(filepath.Join("..", "packages", "bespoke", "trivy.yaml"))
	require.NoError(t, err)
	text := string(recipe)

	// When: its package, source, remediation, smoke, and license metadata are inspected.

	// Then: revision r4 rebuilds immutable Apache-2.0 v0.72.0 source with the fixed toolchain and dependencies.
	require.Contains(t, text, "name: trivy")
	require.Contains(t, text, "version: \"0.72.0\"")
	require.Contains(t, text, "epoch: 4")
	require.Contains(t, text, "license: Apache-2.0")
	require.Contains(t, text, "expected-commit: 8a32853686209a428179bb3a1688802b25691564")
	require.Contains(t, text, "repository: https://github.com/aquasecurity/trivy")
	require.Contains(t, text, "tag: v${{package.version}}")
	require.Contains(t, text, "github.com/sigstore/cosign/v2@v2.6.3")
	require.Contains(t, text, "github.com/sigstore/sigstore-go@v1.2.0")
	require.Contains(t, text, "github.com/sigstore/timestamp-authority/v2@v2.1.2")
	require.Contains(t, text, "go.opentelemetry.io/otel@v1.44.0")
	require.Contains(t, text, "oras.land/oras-go/v2@v2.6.2")
	require.Contains(t, text, "golang.org/x/net@v0.56.0")
	require.Contains(t, text, "golang.org/x/text@v0.39.0")
	require.Contains(t, text, "go-package: go-1.26")
	require.Contains(t, text, "packages: ./cmd/trivy")
	require.Contains(t, text, "output: trivy")
	require.Contains(t, text, "trivy version -f json")
	require.Contains(t, text, "trivy config")
	require.Contains(t, text, "busybox")
	require.Contains(t, text, "trivy=${{package.full-version}}")
	require.Contains(t, text, "apk info --who-owns /usr/bin/trivy")
	require.Contains(t, text, "apk info --license trivy")
	require.Contains(t, text, "usr/share/licenses/${{package.name}}/LICENSE")
}

func Test_Trivy_resolves_fixed_local_package(t *testing.T) {
	// Given: the checked-in image template and package produced by the bespoke recipe.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "trivy.yaml"))
	require.NoError(t, err)
	tmpl := def.Types["default"]

	// When: the local Melange artifact is pinned for the image build.
	err = pinLocalPackageVersions(&tmpl, "0", []apkindex.Package{
		{Name: "trivy", Version: "0.72.0-r4"},
	})

	// Then: apko can only select the approved fixed locally built revision.
	require.NoError(t, err)
	require.Equal(t, []string{"trivy=0.72.0-r4@local"}, tmpl.Packages)
}

func Test_Trivy_CI_uses_reusable_image_security_gate(t *testing.T) {
	// Given: the repository tool pin and Integer publish workflow.
	toolConfig, err := os.ReadFile(filepath.Join("..", "mise.toml"))
	require.NoError(t, err)
	workflow := readGeneratedWorkflowFixture(t, "integer-build-image-reusable.yaml")

	// When: the Trivy-family scan gate is inspected.

	// Then: CI records the independent pinned scanner before it validates the produced image.
	require.Contains(t, string(toolConfig), "trivy = \"0.72.0\"")
	requireIntegerImageReusableImageGate(t, workflow)
}
