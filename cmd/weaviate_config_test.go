package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/integer/apkindex"
	intconfig "github.com/verity-org/verity/internal/integer/config"
)

func Test_Weaviate_uses_pinned_bespoke_package(t *testing.T) {
	// Given: the checked-in Weaviate image definition.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "weaviate.yaml"))
	require.NoError(t, err)

	// When: the approved default image variant is resolved.
	tmpl := def.Types["default"]

	// Then: the image selects the local rebuild and preserves its runtime contract.
	require.NotNil(t, tmpl.Melange)
	require.Equal(t, "weaviate.yaml", tmpl.Melange.Bespoke.First())
	require.Equal(t, []string{"weaviate"}, tmpl.Packages)
	require.Equal(t, "/usr/bin/weaviate", tmpl.Entrypoint)
}

func Test_Weaviate_recipe_pins_immutable_fixed_source(t *testing.T) {
	// Given: the bespoke package recipe selected by the image.
	recipe, err := os.ReadFile(filepath.Join("..", "packages", "bespoke", "weaviate.yaml"))
	require.NoError(t, err)
	text := string(recipe)

	// When: its source, dependencies, runtime, health, and license metadata are inspected.

	// Then: revision r1 rebuilds immutable BSD-3-Clause v1.38.4 source with every known fix.
	require.Contains(t, text, "name: weaviate")
	require.Contains(t, text, "version: \"1.38.4\"")
	require.Contains(t, text, "epoch: 1")
	require.Contains(t, text, "license: BSD-3-Clause")
	require.Contains(t, text, "repository: https://github.com/weaviate/weaviate")
	require.Contains(t, text, "tag: v${{package.version}}")
	require.Contains(t, text, "expected-commit: 6af189f84eed9fcc9ea24614ed0b52bcd6d01abb")
	require.Contains(t, text, "CVE-2026-2303")
	require.Contains(t, text, "CVE-2026-41178")
	require.Contains(t, text, "CVE-2026-42505")
	require.Contains(t, text, "GO-2026-5932")
	require.Contains(t, text, "go.mongodb.org/mongo-driver@v1.17.7")
	require.Contains(t, text, "go.opentelemetry.io/otel@v1.44.0")
	require.Contains(t, text, "go.opentelemetry.io/otel/sdk@v1.44.0")
	require.Contains(t, text, "golang.org/x/crypto@v0.54.0")
	require.Contains(t, text, "golang.org/x/net@v0.57.0")
	require.Contains(t, text, "golang.org/x/text@v0.40.0")
	require.Contains(t, text, "rm -rf .verity-xcrypto/openpgp")
	require.Contains(t, text, "go mod edit -replace=golang.org/x/crypto=./.verity-xcrypto")
	require.Contains(t, text, "go-package: go-1.26")
	require.Contains(t, text, "packages: ./cmd/weaviate-server")
	require.Contains(t, text, "go version -m")
	require.Contains(t, text, "go1\\.26\\.([5-9]|[1-9][0-9]+)")
	require.Contains(t, text, "weaviate --help")
	require.Contains(t, text, "test/daemon-check-output")
	require.Contains(t, text, "curl -fsS")
	require.Contains(t, text, "well-known/ready")
	require.Contains(t, text, "/v1/meta")
	require.Contains(t, text, "--arg version \"${{package.version}}\"")
	require.Contains(t, text, ".version == $version")
	require.Contains(t, text, "apk info --who-owns /usr/bin/weaviate")
	require.Contains(t, text, "apk info --license weaviate")
	require.Contains(t, text, "usr/share/licenses/${{package.name}}/LICENSE")
	require.Contains(t, text, "spdx.json")
	require.Contains(t, text, "--arg full_version \"${{package.full-version}}\"")
	require.Contains(t, text, ".versionInfo == $full_version")
	require.Contains(t, text, "licenseDeclared == \"BSD-3-Clause\"")
}

func Test_Weaviate_resolves_fixed_local_package(t *testing.T) {
	// Given: the checked-in image template and package produced by the bespoke recipe.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "weaviate.yaml"))
	require.NoError(t, err)
	tmpl := def.Types["default"]

	// When: the local Melange artifact is pinned for the image build.
	err = pinLocalPackageVersions(&tmpl, "1", []apkindex.Package{
		{Name: "weaviate", Version: "1.38.4-r1"},
	})

	// Then: apko can only select the approved fixed locally built revision.
	require.NoError(t, err)
	require.Equal(t, []string{"weaviate=1.38.4-r1@local"}, tmpl.Packages)
}

func Test_Weaviate_CI_tests_package_natively(t *testing.T) {
	// Given: the Integer package workflow used by both architecture runners.
	workflow, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "integer-build-image.yaml"))
	require.NoError(t, err)
	text := string(workflow)

	// When: the Weaviate-family package test gate is inspected.

	// Then: each native package leg runs the recipe's runtime and provenance tests.
	require.Contains(t, text, "Test Weaviate package natively (${{ matrix.arch }})")
	require.Contains(t, text, "if: inputs.image == 'weaviate'")
	require.Contains(t, text, "--pipeline-dirs melange-work/pipelines")
	require.Contains(t, text, "melange-work/specs/weaviate.yaml/build.yaml")
}

func Test_Weaviate_CI_tests_image_natively(t *testing.T) {
	// Given: the Integer package workflow used by both architecture runners.
	workflow, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "integer-build-image.yaml"))
	require.NoError(t, err)
	text := string(workflow)

	// When: the Weaviate-family native image gate is inspected.

	// Then: each native runner builds, scans, starts, and validates SPDX for its image.
	require.Contains(t, text, "Test Weaviate image natively (${{ matrix.arch }})")
	require.Contains(t, text, "--fail-on-severity UNKNOWN,LOW,MEDIUM,HIGH,CRITICAL")
	require.Contains(t, text, "docker run --detach --rm --name")
	require.Contains(t, text, "/v1/.well-known/ready")
	require.Contains(t, text, "weaviate-1.38.4-r1.spdx.json")
	require.Contains(t, text, "licenseDeclared == \"BSD-3-Clause\"")
}

func Test_Weaviate_PR_smoke_tests_runtime_and_provenance_natively(t *testing.T) {
	// Given: the pull-request smoke workflow used by both architecture runners.
	workflow, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "pr-test.yaml"))
	require.NoError(t, err)
	text := string(workflow)

	// When: the Weaviate-family native smoke gate is inspected.

	// Then: PR CI runs package tests plus bounded image health and SPDX validation.
	require.Contains(t, text, "if [ \"$image\" = \"weaviate\" ]; then")
	require.Contains(t, text, "expected_version=$(yq -r '.package.version'")
	require.Contains(t, text, "expected_full_version=$(yq -r '.package.version + \"-r\" + (.package.epoch | tostring)'")
	require.Contains(t, text, "melange test \\")
	require.Contains(t, text, "/v1/.well-known/ready")
	require.Contains(t, text, "jq -e --arg version \"$expected_version\"")
	require.Contains(t, text, ".version == $version")
	require.NotContains(t, text, "docker exec \"$container\" /usr/bin/weaviate --help")
	require.Contains(t, text, "docker exec \"$container\" id -u | grep -Fx 65532")
	require.Contains(t, text, "trap 'docker rm --force \"$container\"")
	require.Contains(t, text, "weaviate-${expected_full_version}.spdx.json")
	require.Contains(t, text, "--arg full_version \"$expected_full_version\"")
	require.Contains(t, text, ".spdxVersion == \"SPDX-2.3\"")
	require.Contains(t, text, ".versionInfo == $full_version")
	require.Contains(t, text, "licenseDeclared == \"BSD-3-Clause\"")
}
