package melange

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestNativePackageRecipes_includeInvokedTestTools(t *testing.T) {
	// Given: native package recipes whose test pipelines invoke external tools.
	tests := []struct {
		recipe string
		tool   string
	}{
		{recipe: "velero.yaml", tool: "apk-tools"},
		{recipe: "thanos.yaml", tool: "apk-tools"},
		{recipe: "tempo-2.10.yaml", tool: "wget"},
		{recipe: "hydra.yaml", tool: "apk-tools"},
		{recipe: "external-secrets-operator.yaml", tool: "apk-tools"},
		{recipe: "kor.yaml", tool: "apk-tools"},
	}
	paths := repositoryTestPaths(t)

	for _, test := range tests {
		t.Run(test.recipe, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(paths.BespokeDir, test.recipe))
			require.NoError(t, err)
			var recipe struct {
				Test struct {
					Environment struct {
						Contents struct {
							Packages []string `yaml:"packages"`
						} `yaml:"contents"`
					} `yaml:"environment"`
				} `yaml:"test"`
			}

			// When: the Melange test environment is parsed.
			require.NoError(t, yaml.Unmarshal(data, &recipe))

			// Then: every command invoked by the test is installed explicitly.
			require.Contains(t, recipe.Test.Environment.Contents.Packages, test.tool)
		})
	}
}

func TestCiliumCompatRecipe_includesAPKToolsForVirtualPackageTest(t *testing.T) {
	// Given: the locked Cilium recipe containing the compatibility subpackage.
	paths := repositoryTestPaths(t)
	data, err := os.ReadFile(filepath.Join(paths.LockedDir, "cilium-1.19.yaml"))
	require.NoError(t, err)
	var recipe struct {
		Subpackages []struct {
			Name string `yaml:"name"`
			Test struct {
				Environment struct {
					Contents struct {
						Packages []string `yaml:"packages"`
					} `yaml:"contents"`
				} `yaml:"environment"`
			} `yaml:"test"`
		} `yaml:"subpackages"`
	}
	require.NoError(t, yaml.Unmarshal(data, &recipe))

	// When: the compatibility subpackage test environment is selected.
	var packages []string
	for _, subpackage := range recipe.Subpackages {
		if subpackage.Name == "${{package.name}}-compat" {
			packages = subpackage.Test.Environment.Contents.Packages
			break
		}
	}

	// Then: the reusable virtual-package test can invoke apk explicitly.
	require.Contains(t, packages, "apk-tools")
}

func TestCraneRecipe_retainsCoverageFloorWithConfigProbe(t *testing.T) {
	// Given: the locked Crane recipe and its xcover gate.
	paths := repositoryTestPaths(t)
	data, err := os.ReadFile(filepath.Join(paths.LockedDir, "crane.yaml"))
	require.NoError(t, err)

	// When: the functional coverage probes are inspected.
	text := string(data)

	// Then: config retrieval raises exercised coverage without lowering the floor.
	require.Contains(t, text, `crane config chainguard/static:latest | jq -e '.architecture != null'`)
	require.Contains(t, text, "min-coverage: 17")
}
