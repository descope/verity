package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/integer/apkindex"
	intconfig "github.com/verity-org/verity/internal/integer/config"
)

func Test_SealedSecrets_resolves_only_owned_local_package_when_versions_are_pinned(t *testing.T) {
	// Given: the Sealed Secrets image template and its family-owned local package.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "sealed-secrets.yaml"))
	require.NoError(t, err)
	tmpl := def.Types["default"]

	// When: the dual-architecture package indexes report the approved local revision.
	err = pinLocalPackageVersions(&tmpl, "0", []apkindex.Package{
		{Name: "sealed-secrets-0", Version: "0.38.4-r0"},
	})

	// Then: apko cannot fall back to the generic upstream Sealed Secrets package.
	require.NoError(t, err)
	require.Equal(t, []string{"sealed-secrets-0=0.38.4-r0@local"}, tmpl.Packages)
}

func Test_SealedSecrets_CI_runs_native_package_image_runtime_spdx_and_strict_trivy_gates(t *testing.T) {
	// Given: the scripts and workflows shared by the native architecture runners.
	packageScript, err := os.ReadFile(filepath.Join("..", ".github", "scripts", "test-sealed-secrets-package.sh"))
	require.NoError(t, err)
	imageScript, err := os.ReadFile(filepath.Join("..", ".github", "scripts", "test-sealed-secrets-image.sh"))
	require.NoError(t, err)
	prWorkflow, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "pr-test.yaml"))
	require.NoError(t, err)

	// When: the Sealed Secrets family gates are inspected.
	packageText := string(packageScript)
	imageText := string(imageScript)
	prText := string(prWorkflow)
	buildText := readGeneratedWorkflowFixture(t, "integer-build-image-reusable.yaml")

	// Then: both native legs execute package and image proof before accepting zero findings.
	require.Contains(t, packageText, "case \"$arch\" in")
	require.Contains(t, packageText, "timeout --signal=TERM --kill-after=1m 30m melange test")
	require.Contains(t, packageText, "melange-work/specs/sealed-secrets-0.yaml/build.yaml")
	require.Contains(t, imageText, "/usr/bin/controller")
	require.Contains(t, imageText, "/usr/bin/kubeseal")
	require.Contains(t, imageText, "container=$(docker create \"$image_ref\")")
	require.Contains(t, imageText, "if [[ -n \"$container\" ]]")
	require.Contains(t, imageText, "ca-certificates-bundle")
	require.Contains(t, imageText, ".spdxVersion == \"SPDX-2.3\"")
	require.Contains(t, imageText, "licenseDeclared == \"Apache-2.0\"")
	requireIntegerImageReusablePackageGate(t, buildText)
	requireIntegerImageReusableImageGate(t, buildText)
	require.Contains(t, prText, "./verity ci pr-test integer-batch")
	require.NotContains(t, prText, "test-sealed-secrets-package.sh")
	require.NotContains(t, prText, "test-sealed-secrets-image.sh")
}
