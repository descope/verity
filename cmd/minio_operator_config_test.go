package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/integer/apkindex"
	intconfig "github.com/verity-org/verity/internal/integer/config"
)

func Test_MinioOperator_uses_pinned_bespoke_package(t *testing.T) {
	// Given: the checked-in MinIO Operator image definition.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "minio-operator.yaml"))
	require.NoError(t, err)

	// When: the approved default image variant is resolved.
	tmpl := def.Types["default"]

	// Then: the image selects the local rebuild and preserves its runtime contract.
	require.NotNil(t, tmpl.Melange)
	require.Equal(t, "minio-operator.yaml", tmpl.Melange.Bespoke.First())
	require.Equal(t, []string{"minio-operator"}, tmpl.Packages)
	require.Equal(t, "/usr/bin/minio-operator", tmpl.Entrypoint)
	require.Contains(t, def.Versions, "7")
}

func Test_MinioOperator_recipe_pins_fixed_source_revision(t *testing.T) {
	// Given: the bespoke package recipe selected by the image.
	recipe, err := os.ReadFile(filepath.Join("..", "packages", "bespoke", "minio-operator.yaml"))
	require.NoError(t, err)
	text := string(recipe)

	// When: its package, source, dependency, build, test, and license metadata are inspected.

	// Then: direct revision r28 rebuilds immutable AGPL-3.0-only v7.1.1 source with fixed Prometheus.
	require.Contains(t, text, "name: minio-operator")
	require.Contains(t, text, "# renovate: datasource=github-tags depName=minio/operator versioning=semver-coerced")
	require.Contains(t, text, "version: \"7.1.1\"")
	require.Contains(t, text, "epoch: 28")
	require.Contains(t, text, "license: AGPL-3.0-only")
	require.Contains(t, text, "Adapted from wolfi-dev/os@58777d20dc737bd1039fb43c3896604b14e4cb71")
	require.Contains(t, text, "6eee6a7caa70555ad009e522ce04861297e9e2be.tar.gz")
	require.Contains(t, text, "expected-sha256: 5a851e8e51fc235ece86bd4de87ddcf38b41c387b9798b8770960b865b3d654a")
	require.Contains(t, text, "github.com/prometheus/prometheus@v0.311.3")
	require.Contains(t, text, "github.com/go-openapi/testify/enable/yaml/v2@v2.4.0")
	require.Contains(t, text, "go-package: go-1.26")
	require.Contains(t, text, "packages: ./cmd/operator")
	require.Contains(t, text, "output: minio-operator")
	require.Contains(t, text, "github.com/minio/operator/pkg.Version=${{package.full-version}}")
	require.Contains(t, text, "github.com/minio/operator/pkg.ShortCommitID=6eee6a7c\n")
	require.Contains(t, text, "usr/share/licenses/${{package.name}}/LICENSE")
	require.Contains(t, text, "minio-operator --version")
	require.Contains(t, text, "go version -m /usr/bin/minio-operator")
	require.Contains(t, text, "apk info --who-owns /usr/bin/minio-operator")
}

func Test_MinioOperator_resolves_fixed_local_package(t *testing.T) {
	// Given: the checked-in image template and the package produced by the bespoke recipe.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "minio-operator.yaml"))
	require.NoError(t, err)
	tmpl := def.Types["default"]

	// When: the local Melange artifact is pinned for the image build.
	err = pinLocalPackageVersions(&tmpl, "7", []apkindex.Package{{
		Name:    "minio-operator",
		Version: "7.1.1-r28",
	}})

	// Then: apko can only select the approved fixed locally built revision.
	require.NoError(t, err)
	require.Equal(t, []string{"minio-operator=7.1.1-r28@local"}, tmpl.Packages)
}
