package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	intconfig "github.com/verity-org/verity/internal/integer/config"
)

func TestTerraformDefaultsUseTruthfulStreamRecipes(t *testing.T) {
	repoRoot, err := filepath.Abs("..")
	require.NoError(t, err)

	definition, err := intconfig.LoadImage(filepath.Join(repoRoot, "images", "terraform.yaml"))
	require.NoError(t, err)

	defaultType := definition.Types["default"]
	assert.Equal(t, []string{"verity-opentofu-{{version}}"}, defaultType.Packages)
	require.NotNil(t, defaultType.Melange)
	assert.Equal(t, intconfig.StringList{"opentofu-{{version}}.yaml"}, defaultType.Melange.Bespoke)

	expected := map[string]struct {
		version string
		commit  string
	}{
		"1.10": {version: "1.10.10", commit: "51257d7593a2a90bdaa8c0ed15ed967fe1ff6cbb"},
		"1.11": {version: "1.11.12", commit: "746f6945d14063cca491742ad950a42eb8378a1e"},
		"1.12": {version: "1.12.4", commit: "d5291190380d16aa5ef4590facbe0719d7fa165f"},
	}

	for stream, want := range expected {
		t.Run(stream, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(repoRoot, "packages", "bespoke", "opentofu-"+stream+".yaml"))
			require.NoError(t, err)

			var recipe struct {
				Package struct {
					Name      string `yaml:"name"`
					Version   string `yaml:"version"`
					Copyright []struct {
						License string `yaml:"license"`
					} `yaml:"copyright"`
				} `yaml:"package"`
				Pipeline []struct {
					Uses string            `yaml:"uses"`
					With map[string]string `yaml:"with"`
				} `yaml:"pipeline"`
			}
			require.NoError(t, yaml.Unmarshal(data, &recipe))

			assert.Equal(t, "verity-opentofu-"+stream, recipe.Package.Name)
			assert.Equal(t, want.version, recipe.Package.Version)
			require.NotEmpty(t, recipe.Package.Copyright)
			assert.Equal(t, "MPL-2.0", recipe.Package.Copyright[0].License)
			require.NotEmpty(t, recipe.Pipeline)
			assert.Equal(t, "git-checkout", recipe.Pipeline[0].Uses)
			assert.Equal(t, "v${{package.version}}", recipe.Pipeline[0].With["tag"])
			assert.Equal(t, want.commit, recipe.Pipeline[0].With["expected-commit"])
		})
	}
}
