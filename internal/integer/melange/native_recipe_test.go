package melange

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

var (
	apkTestCommand  = regexp.MustCompile(`(^|[[:space:];|&()])([^[:space:];|&()]*/)?apk(\s|$)`)
	wgetTestCommand = regexp.MustCompile(`(^|[[:space:];|&()])([^[:space:];|&()]*/)?wget(\s|$)`)
)

func TestNativeTestCommandPatterns_matchExecutablePaths(t *testing.T) {
	tests := []struct {
		name    string
		pattern *regexp.Regexp
		script  string
	}{
		{name: "apk name", pattern: apkTestCommand, script: "apk info package"},
		{name: "apk absolute path", pattern: apkTestCommand, script: "/sbin/apk info package"},
		{name: "apk relative path", pattern: apkTestCommand, script: "./tools/apk info package"},
		{name: "wget name", pattern: wgetTestCommand, script: "wget -qO- http://127.0.0.1"},
		{name: "wget absolute path", pattern: wgetTestCommand, script: "/usr/bin/wget -qO- http://127.0.0.1"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.True(t, test.pattern.MatchString(test.script))
		})
	}
}

func TestNativePackageRecipes_includeInvokedTestTools(t *testing.T) {
	// Given: every native package recipe and its isolated Melange tests.
	paths := repositoryTestPaths(t)
	var recipes []string
	require.NoError(t, filepath.WalkDir(paths.BespokeDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && filepath.Ext(path) == ".yaml" {
			recipes = append(recipes, path)
		}
		return nil
	}))

	for _, path := range recipes {
		relative, err := filepath.Rel(paths.BespokeDir, path)
		require.NoError(t, err)
		t.Run(relative, func(t *testing.T) {
			data, readErr := os.ReadFile(path)
			require.NoError(t, readErr)
			var recipe nativeRecipeTestContract
			require.NoError(t, yaml.Unmarshal(data, &recipe))

			// When: every top-level and subpackage test pipeline is inspected.
			requireNativeTestTools(t, "package", recipe.Test)
			for _, subpackage := range recipe.Subpackages {
				requireNativeTestTools(t, subpackage.Name, subpackage.Test)
			}
		})
	}
}

type nativeRecipeTestContract struct {
	Test        nativeTestContract `yaml:"test"`
	Subpackages []struct {
		Name string             `yaml:"name"`
		Test nativeTestContract `yaml:"test"`
	} `yaml:"subpackages"`
}

type nativeTestContract struct {
	Environment struct {
		Contents struct {
			Packages []string `yaml:"packages"`
		} `yaml:"contents"`
	} `yaml:"environment"`
	Pipeline []struct {
		Uses string `yaml:"uses"`
		Runs string `yaml:"runs"`
	} `yaml:"pipeline"`
}

func requireNativeTestTools(t *testing.T, name string, test nativeTestContract) {
	t.Helper()
	for _, step := range test.Pipeline {
		if apkTestCommand.MatchString(step.Runs) || step.Uses == "test/virtualpackage" || step.Uses == "test/emptypackage" {
			assert.Contains(t, test.Environment.Contents.Packages, "apk-tools", name)
		}
		if wgetTestCommand.MatchString(step.Runs) {
			assert.Contains(t, test.Environment.Contents.Packages, "wget", name)
		}
	}
}

func TestCiliumRecipe_includesAPKToolsForEveryAPKInspectingSubpackageTest(t *testing.T) {
	// Given: the locked Cilium recipe containing reusable tests that invoke apk.
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
				Pipeline []struct {
					Uses string `yaml:"uses"`
				} `yaml:"pipeline"`
			} `yaml:"test"`
		} `yaml:"subpackages"`
	}
	require.NoError(t, yaml.Unmarshal(data, &recipe))

	// When: subpackage tests using APK-inspection pipelines are selected.
	var inspected []string
	for _, subpackage := range recipe.Subpackages {
		for _, step := range subpackage.Test.Pipeline {
			if step.Uses != "test/virtualpackage" && step.Uses != "test/emptypackage" {
				continue
			}
			inspected = append(inspected, subpackage.Name)
			require.Contains(t, subpackage.Test.Environment.Contents.Packages, "apk-tools", subpackage.Name)
		}
	}

	// Then: both current APK-inspection tests remain covered by the contract.
	require.ElementsMatch(t, []string{"${{package.name}}-compat", "${{package.name}}-iptables"}, inspected)
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
