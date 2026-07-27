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
