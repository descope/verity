package melange

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestPinConfigConfiguredLocalPackages(t *testing.T) {
	_, testFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(testFile), "..", "..", ".."))

	tests := []struct {
		image         string
		version       string
		imageType     string
		packageName   string
		pinnedPackage string
	}{
		{image: "loki", version: "3.7", imageType: "default", packageName: "loki-3.7", pinnedPackage: "loki-3.7=3.7.3-r99@local"},
		{image: "haproxy", version: "3.2", imageType: "default", packageName: "haproxy-3.2", pinnedPackage: "haproxy-3.2=3.2.21-r0@local"},
		{image: "haproxy", version: "3.2", imageType: "fips", packageName: "haproxy-3.2", pinnedPackage: "haproxy-3.2=3.2.21-r0@local"},
		{image: "haproxy", version: "3.3", imageType: "default", packageName: "haproxy-3.3", pinnedPackage: "haproxy-3.3=3.3.12-r0@local"},
		{image: "haproxy", version: "3.3", imageType: "fips", packageName: "haproxy-3.3", pinnedPackage: "haproxy-3.3=3.3.12-r0@local"},
	}

	for _, test := range tests {
		t.Run(test.image+"-"+test.version+"-"+test.imageType, func(t *testing.T) {
			spec, err := ResolveSpec(filepath.Join(repoRoot, "images"), test.image, test.version, test.imageType)
			require.NoError(t, err)
			require.NotEmpty(t, spec.Bespoke)

			root := t.TempDir()
			configPath := filepath.Join(root, "config.apko.yaml")
			repositoryDir := filepath.Join(root, "repo")
			writeTestFile(t, configPath, "contents:\n  packages: ["+test.packageName+"]\n")
			index := configuredBespokeIndex(t, filepath.Join(repoRoot, "packages", "bespoke"), spec.Bespoke)
			writeAPKIndexArchive(t, filepath.Join(repositoryDir, "x86_64", "APKINDEX.tar.gz"), index)
			writeAPKIndexArchive(t, filepath.Join(repositoryDir, "aarch64", "APKINDEX.tar.gz"), index)

			err = PinConfigPackages(PinConfigOptions{
				RootDir:       root,
				ConfigPath:    configPath,
				RepositoryDir: repositoryDir,
				Architectures: []Architecture{ArchitectureX8664, ArchitectureAArch64},
			})

			require.NoError(t, err)
			assert.Equal(t, []string{test.pinnedPackage}, readConfigPackages(t, configPath))
		})
	}
}

func configuredBespokeIndex(t *testing.T, bespokeDir string, recipes []string) string {
	t.Helper()
	var index strings.Builder
	for _, recipe := range recipes {
		data, err := os.ReadFile(filepath.Join(bespokeDir, recipe))
		require.NoError(t, err)
		var document struct {
			Package struct {
				Name    string `yaml:"name"`
				Version string `yaml:"version"`
				Epoch   int    `yaml:"epoch"`
			} `yaml:"package"`
		}
		require.NoError(t, yaml.Unmarshal(data, &document))
		require.NotEmpty(t, document.Package.Name)
		require.NotEmpty(t, document.Package.Version)
		fmt.Fprintf(&index, "P:%s\nV:%s-r%d\n\n", document.Package.Name, document.Package.Version, document.Package.Epoch)
	}
	return index.String()
}
