package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/integer/apkindex"
	intconfig "github.com/verity-org/verity/internal/integer/config"
)

func Test_Pushgateway_uses_fixed_bespoke_package_and_preserves_alias(t *testing.T) {
	// Given: the checked-in Pushgateway image definition.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "pushgateway.yaml"))
	require.NoError(t, err)

	// When: the only supported image variant and version family are resolved.
	tmpl := def.Types["default"]

	// Then: the image selects the local rebuild without changing the 1.11 alias.
	require.Len(t, def.Types, 1)
	require.Len(t, def.Versions, 1)
	require.Contains(t, def.Versions, "1.11")
	require.NotNil(t, tmpl.Melange)
	require.Equal(t, "pushgateway.yaml", tmpl.Melange.Bespoke.First())
	require.Equal(t, []string{"prometheus-pushgateway"}, tmpl.Packages)
	require.Equal(t, "/usr/bin/pushgateway", tmpl.Entrypoint)
}

func Test_Pushgateway_recipe_pins_immutable_fixed_source_and_provenance(t *testing.T) {
	// Given: the bespoke package recipe selected by the image.
	recipe, err := os.ReadFile(filepath.Join("..", "packages", "bespoke", "pushgateway.yaml"))
	require.NoError(t, err)
	text := string(recipe)

	// When: its source, fixed revision, runtime smoke, and license metadata are inspected.

	// Then: r2 rebuilds immutable Apache-2.0 v1.11.3 source containing both Prometheus fixes.
	require.Contains(t, text, "name: prometheus-pushgateway")
	require.Contains(t, text, "version: \"1.11.3\"")
	require.Contains(t, text, "epoch: 2")
	require.Contains(t, text, "license: Apache-2.0")
	require.Contains(t, text, "Adapted from wolfi-dev/os@c18151bd0f768439d4e969ec5472e6844f33cc70")
	require.Contains(t, text, "repository: https://github.com/prometheus/pushgateway")
	require.Contains(t, text, "tag: v${{package.version}}")
	require.Contains(t, text, "expected-commit: e803ebd81be5867ff17a21205030611fa033af13")
	require.Contains(t, text, "CVE-2026-42151")
	require.Contains(t, text, "CVE-2026-42154")
	require.Contains(t, text, "go-package: go-1.26")
	require.Contains(t, text, "GOTOOLCHAIN: local")
	require.NotContains(t, text, "tags: netgo,osusergo")
	require.Contains(t, text, "github.com/prometheus/prometheus[[:space:]]+v0.311.3")
	require.Contains(t, text, "pushgateway --version")
	require.Contains(t, text, "test/daemon-check-output")
	require.Contains(t, text, "/-/ready")
	require.Contains(t, text, "/metrics/job/integer_pushgateway_smoke")
	require.Contains(t, text, "integer_pushgateway_smoke 7")
	require.Contains(t, text, "apk info --license prometheus-pushgateway | grep -Fx Apache-2.0")
	require.Contains(t, text, "usr/share/licenses/${{package.name}}/LICENSE")
	require.Contains(t, text, "spdx.json")
	require.Contains(t, text, "licenseDeclared == \"Apache-2.0\"")
	require.NotContains(t, text, "subpackages:")
}

func Test_Pushgateway_resolves_only_fixed_local_package(t *testing.T) {
	// Given: the checked-in image template and locally built Pushgateway package.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "pushgateway.yaml"))
	require.NoError(t, err)
	tmpl := def.Types["default"]

	// When: the local Melange artifact is pinned for the image build.
	err = pinLocalPackageVersions(&tmpl, "1.11", []apkindex.Package{{
		Name:    "prometheus-pushgateway",
		Version: "1.11.3-r2",
	}})

	// Then: apko can only select the approved fixed local revision.
	require.NoError(t, err)
	require.Equal(t, []string{"prometheus-pushgateway=1.11.3-r2@local"}, tmpl.Packages)
}

func Test_Pushgateway_CI_verifies_package_and_image_natively(t *testing.T) {
	// Given: the native package and image workflow used for publish builds.
	workflow, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "integer-build-image.yaml"))
	require.NoError(t, err)
	text := string(workflow)

	// When: the Pushgateway-family gates are inspected.

	// Then: both native legs test the package and image with strict security and provenance checks.
	require.Contains(t, text, "Test Pushgateway package natively (${{ matrix.arch }})")
	require.Contains(t, text, "Test Pushgateway image natively (${{ matrix.arch }})")
	require.Contains(t, text, "if: inputs.image == 'pushgateway'")
	require.Contains(t, text, "melange-work/specs/pushgateway.yaml/build.yaml")
	require.Contains(t, text, "--fail-on-severity UNKNOWN,LOW,MEDIUM,HIGH,CRITICAL")
	require.Contains(t, text, "/-/ready")
	require.Contains(t, text, "/metrics/job/integer_pushgateway_smoke")
	require.Contains(t, text, "printf '%s\\n'")
	require.Contains(t, text, "prometheus-pushgateway-${expected_full_version}.spdx.json")
	require.Contains(t, text, "licenseDeclared == \"Apache-2.0\"")
}

func Test_Pushgateway_PR_smoke_verifies_runtime_and_provenance_natively(t *testing.T) {
	// Given: the pull-request smoke workflow used by both architecture runners.
	workflow, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "pr-test.yaml"))
	require.NoError(t, err)
	text := string(workflow)

	// When: the Pushgateway-family native smoke gate is inspected.

	// Then: PR CI runs package tests plus image health, metrics, SPDX, and strict Trivy validation.
	require.Contains(t, text, "if [ \"$image\" = \"pushgateway\" ]; then")
	require.Contains(t, text, "melange test \\")
	require.Contains(t, text, "--fail-on-severity \"UNKNOWN,LOW,MEDIUM,HIGH,CRITICAL\"")
	require.Contains(t, text, "/-/ready")
	require.Contains(t, text, "/metrics/job/integer_pushgateway_smoke")
	require.Contains(t, text, "printf '%s\\n'")
	require.Contains(t, text, "prometheus-pushgateway-${expected_full_version}.spdx.json")
	require.Contains(t, text, ".spdxVersion == \"SPDX-2.3\"")
	require.Contains(t, text, "licenseDeclared == \"Apache-2.0\"")
}
