package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/integer/apkindex"
	intconfig "github.com/verity-org/verity/internal/integer/config"
)

func Test_AWSS3Controller_uses_pinned_bespoke_package(t *testing.T) {
	// Given: the checked-in AWS S3 Controller image definition.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "aws-s3-controller.yaml"))
	require.NoError(t, err)

	// When: the approved default image variant is resolved.
	tmpl := def.Types["default"]

	// Then: the image selects the locally rebuilt package and preserves its runtime contract.
	require.NotNil(t, tmpl.Melange)
	require.Equal(t, "aws-s3-controller.yaml", tmpl.Melange.Bespoke.First())
	require.Equal(t, []string{"aws-s3-controller"}, tmpl.Packages)
	require.Equal(t, "/usr/bin/controller", tmpl.Entrypoint)
}

func Test_AWSS3Controller_recipe_pins_fixed_source_revision(t *testing.T) {
	// Given: the bespoke package recipe selected by the image.
	recipe, err := os.ReadFile(filepath.Join("..", "packages", "bespoke", "aws-s3-controller.yaml"))
	require.NoError(t, err)
	text := string(recipe)

	// When: its package, source, version, and license metadata are inspected.

	// Then: the fixed r0 release is rebuilt from immutable Apache-2.0 source.
	require.Contains(t, text, "name: aws-s3-controller")
	require.Contains(t, text, "version: \"1.8.1\"")
	require.Contains(t, text, "epoch: 0")
	require.Contains(t, text, "license: Apache-2.0")
	require.Contains(t, text, "repository: https://github.com/aws-controllers-k8s/s3-controller.git")
	require.Contains(t, text, "tag: v${{package.version}}")
	require.Contains(t, text, "expected-commit: cba55a118f7ee122c2da08f3d4148d79ac953972")
	require.Contains(t, text, "go-package: go-1.26")
	require.Contains(t, text, "github.com/aws-controllers-k8s/s3-controller/pkg/version.GitVersion")
	require.Contains(t, text, "github.com/aws-controllers-k8s/s3-controller/pkg/version.GitCommit")
	require.Contains(t, text, "github.com/aws-controllers-k8s/s3-controller/pkg/version.BuildDate")
	require.Contains(t, text, "cba55a118f7ee122c2da08f3d4148d79ac953972")
	require.Contains(t, text, "identifier: aws-controllers-k8s/s3-controller")
	require.Contains(t, text, "strip-prefix: v")
	require.Contains(t, text, "use-tag: true")
}

func Test_AWSS3Controller_resolves_fixed_local_package(t *testing.T) {
	// Given: the checked-in image template and the package produced by the bespoke recipe.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "aws-s3-controller.yaml"))
	require.NoError(t, err)
	tmpl := def.Types["default"]

	// When: local Melange artifacts are pinned for the image build.
	err = pinLocalPackageVersions(&tmpl, "1", []apkindex.Package{{
		Name:    "aws-s3-controller",
		Version: "1.8.1-r0",
	}})

	// Then: apko can only select the fixed locally built revision.
	require.NoError(t, err)
	require.Equal(t, []string{"aws-s3-controller=1.8.1-r0@local"}, tmpl.Packages)
}
