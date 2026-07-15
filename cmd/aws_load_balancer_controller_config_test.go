package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/integer/apkindex"
	intconfig "github.com/verity-org/verity/internal/integer/config"
)

func Test_AWSLoadBalancerController_uses_pinned_bespoke_package(t *testing.T) {
	// Given: the checked-in AWS Load Balancer Controller image definition.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "aws-load-balancer-controller.yaml"))
	require.NoError(t, err)

	// When: the approved default image variant is resolved.
	tmpl := def.Types["default"]

	// Then: the image selects the locally rebuilt package and preserves its runtime contract.
	require.NotNil(t, tmpl.Melange)
	require.Equal(t, "aws-load-balancer-controller.yaml", tmpl.Melange.Bespoke.First())
	require.Equal(t, []string{"aws-load-balancer-controller"}, tmpl.Packages)
	require.Equal(t, "/usr/bin/controller", tmpl.Entrypoint)
}

func Test_AWSLoadBalancerController_recipe_pins_fixed_source_revision(t *testing.T) {
	// Given: the bespoke package recipe selected by the image.
	recipe, err := os.ReadFile(filepath.Join("..", "packages", "bespoke", "aws-load-balancer-controller.yaml"))
	require.NoError(t, err)
	text := string(recipe)

	// When: its package and source metadata are inspected.

	// Then: revision r1 is the approved fixed release from immutable Apache-2.0 source.
	require.Contains(t, text, "name: aws-load-balancer-controller")
	require.Contains(t, text, "version: \"3.4.1\"")
	require.Contains(t, text, "epoch: 1")
	require.Contains(t, text, "license: Apache-2.0")
	require.Contains(t, text, "repository: https://github.com/kubernetes-sigs/aws-load-balancer-controller")
	require.Contains(t, text, "tag: v${{package.version}}")
	require.Contains(t, text, "expected-commit: bc8331031ce97cc33d0bc0f85ab696c73942570a")
	require.Contains(t, text, "sigs.k8s.io/aws-load-balancer-controller/pkg/version")
}

func Test_AWSLoadBalancerController_resolves_fixed_local_package(t *testing.T) {
	// Given: the checked-in image template and the package produced by the bespoke recipe.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "aws-load-balancer-controller.yaml"))
	require.NoError(t, err)
	tmpl := def.Types["default"]

	// When: local Melange artifacts are pinned for the image build.
	err = pinLocalPackageVersions(&tmpl, "2", []apkindex.Package{{
		Name:    "aws-load-balancer-controller",
		Version: "3.4.1-r1",
	}})

	// Then: apko can only select the fixed locally built revision.
	require.NoError(t, err)
	require.Equal(t, []string{"aws-load-balancer-controller=3.4.1-r1@local"}, tmpl.Packages)
}
