package melange

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestOpenSSLProviderFIPSRecipe_includesShellInTestEnvironment(t *testing.T) {
	// Given: the shared FIPS provider recipe used by every OpenSSL FIPS image.
	paths := repositoryTestPaths(t)
	data, err := os.ReadFile(filepath.Join(paths.BespokeDir, "openssl-provider-fips.yaml"))
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

	// Then: Melange can start its shell-based test pipeline.
	require.Contains(t, recipe.Test.Environment.Contents.Packages, "busybox")
}
