package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/integer/apkindex"
	intconfig "github.com/verity-org/verity/internal/integer/config"
)

func Test_Rclone_uses_pinned_bespoke_package(t *testing.T) {
	// Given: the checked-in rclone image definition.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "rclone.yaml"))
	require.NoError(t, err)

	// When: the only supported image variant is resolved.
	tmpl := def.Types["default"]

	// Then: the image selects only the local rebuild and preserves its runtime contract.
	require.Len(t, def.Types, 1)
	require.NotNil(t, tmpl.Melange)
	require.Equal(t, "rclone.yaml", tmpl.Melange.Bespoke.First())
	require.Equal(t, []string{"rclone"}, tmpl.Packages)
	require.Equal(t, "/usr/bin/rclone", tmpl.Entrypoint)
}

func Test_Rclone_recipe_pins_latest_immutable_fixed_source(t *testing.T) {
	// Given: the bespoke package recipe selected by the image.
	recipe, err := os.ReadFile(filepath.Join("..", "packages", "bespoke", "rclone.yaml"))
	require.NoError(t, err)
	text := string(recipe)

	// When: its source, dependency fix, smoke tests, and provenance are inspected.

	// Then: v1.74.4 is immutable, MIT-licensed, and links the fixed x/image release.
	require.Contains(t, text, "Adapted from wolfi-dev/os@cfc19ff702dda08c0d9fef924b0ccdf2009c2698")
	require.Contains(t, text, "name: rclone")
	require.Contains(t, text, "version: \"1.74.4\"")
	require.Contains(t, text, "epoch: 0")
	require.Contains(t, text, "license: MIT")
	require.Contains(t, text, "repository: https://github.com/rclone/rclone")
	require.Contains(t, text, "tag: v${{package.version}}")
	require.Contains(t, text, "expected-commit: 5bc93a2a7ab0ebd0a11352bc4968eabeffb18027")
	require.Contains(t, text, "CVE-2026-33813")
	require.Contains(t, text, "CVE-2026-41178")
	require.Contains(t, text, "CVE-2026-46601")
	require.Contains(t, text, "CVE-2026-46602")
	require.Contains(t, text, "CVE-2026-46604")
	require.Contains(t, text, "go.opentelemetry.io/otel@v1.44.0")
	require.Contains(t, text, "go.opentelemetry.io/otel/metric@v1.44.0")
	require.Contains(t, text, "go.opentelemetry.io/otel/trace@v1.44.0")
	require.Contains(t, text, "golang.org/x/image@v0.43.0")
	require.Contains(t, text, "go-1.26=1.26.5-r1")
	require.Contains(t, text, "go version -m")
	require.Contains(t, text, "rclone --version")
	require.Contains(t, text, "rclone copy")
	require.Contains(t, text, "rclone check --checksum")
	require.Contains(t, text, "- coreutils")
	require.Contains(t, text, "sha256sum --check")
	require.Contains(t, text, "apk info --who-owns /usr/bin/rclone")
	require.Contains(t, text, "apk info --license rclone")
	require.Contains(t, text, "usr/share/licenses/${{package.name}}/COPYING")
	require.Contains(t, text, "spdx.json")
	require.Contains(t, text, "licenseDeclared == \"MIT\"")
}

func Test_Rclone_resolves_fixed_local_package(t *testing.T) {
	// Given: the checked-in image template and package produced by the bespoke recipe.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "rclone.yaml"))
	require.NoError(t, err)
	tmpl := def.Types["default"]

	// When: the local Melange artifact is pinned for the image build.
	err = pinLocalPackageVersions(&tmpl, "1", []apkindex.Package{
		{Name: "rclone", Version: "1.74.4-r0"},
	})

	// Then: apko can only select the approved fixed locally built revision.
	require.NoError(t, err)
	require.Equal(t, []string{"rclone=1.74.4-r0@local"}, tmpl.Packages)
}

func Test_Rclone_CI_uses_reusable_image_security_gates(t *testing.T) {
	// Given: the generated reusable Integer image workflow fixture.
	workflow := readGeneratedWorkflowFixture(t, "integer-build-image-reusable.yaml")

	// When: the shared package and image gates are inspected.

	// Then: reusable execution keeps native package testing, strict scanning, signing, and SBOM attestation.
	requireIntegerImageReusablePackageGate(t, workflow)
	requireIntegerImageReusableImageGate(t, workflow)
}

func Test_Rclone_reusable_image_proof_uses_generated_metadata(t *testing.T) {
	// Given: the generated reusable Integer image workflow fixture.
	workflow := readGeneratedWorkflowFixture(t, "integer-build-image-reusable.yaml")

	// When: the generic image metadata and publish path are inspected.

	// Then: package-version-sensitive proof is delegated to the generated metadata path, without a stale rclone literal.
	requireIntegerImageReusableImageGate(t, workflow)
	require.Contains(t, workflow, "Read image metadata")
	require.Contains(t, workflow, "./verity integer metadata")
	require.NotContains(t, workflow, "rclone-1.74.4-r0.spdx.json")
	require.NotContains(t, workflow, ".versionInfo == \"1.74.4-r0\"")
}

func Test_Rclone_PR_smoke_tests_runtime_copy_checksum_and_provenance_natively(t *testing.T) {
	// Given: the pull-request smoke workflow used by both architecture runners.
	workflow, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "pr-test.yaml"))
	require.NoError(t, err)
	text := string(workflow)

	// When: the rclone-family native smoke gate is inspected.

	// Then: PR CI delegates the native runtime and provenance proof to Go.
	require.Contains(t, text, "./verity ci pr-test integer-batch")
	require.Contains(t, text, "--kind smoke")
	require.NotContains(t, text, "if [ \"$image\" = \"rclone\" ]; then")
}
