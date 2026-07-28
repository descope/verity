package melange

import (
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/syntax"
)

var melangeTemplateExpression = regexp.MustCompile(`\$\{\{[^{}]+\}\}`)

func TestNativeTestCommandDetection_handlesShellSyntax(t *testing.T) {
	tests := []struct {
		name    string
		command string
		script  string
		want    bool
	}{
		{name: "apk name", command: "apk", script: "apk info package", want: true},
		{name: "apk absolute path", command: "apk", script: "/sbin/apk info package", want: true},
		{name: "apk relative path", command: "apk", script: "./tools/apk info package", want: true},
		{name: "apk semicolon delimiter", command: "apk", script: "apk; next", want: true},
		{name: "apk pipe delimiter", command: "apk", script: "apk|next", want: true},
		{name: "apk closing group delimiter", command: "apk", script: "(apk)", want: true},
		{name: "apk and delimiter", command: "apk", script: "apk&&next", want: true},
		{name: "apk redirection delimiter", command: "apk", script: "apk>/tmp/out", want: true},
		{name: "apk quoted absolute path", command: "apk", script: `"/sbin/apk" info package`, want: true},
		{name: "apk quoted relative path", command: "apk", script: `'./tools/apk' info package`, want: true},
		{name: "apk argument", command: "apk", script: "echo apk info", want: false},
		{name: "apk quoted argument", command: "apk", script: `printf '%s\n' "/sbin/apk info"`, want: false},
		{name: "apk prefix", command: "apk", script: "apktool info", want: false},
		{name: "wget name", command: "wget", script: "wget -qO- http://127.0.0.1", want: true},
		{name: "wget absolute path", command: "wget", script: "/usr/bin/wget -qO- http://127.0.0.1", want: true},
		{name: "wget relative path", command: "wget", script: "./tools/wget -qO- http://127.0.0.1", want: true},
		{name: "wget argument", command: "wget", script: "echo wget -qO-", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := nativeTestInvokes(test.script, test.command)
			require.NoError(t, err)
			assert.Equal(t, test.want, got)
		})
	}
}

func nativeTestInvokes(script, command string) (bool, error) {
	normalized := melangeTemplateExpression.ReplaceAllString(script, "melange")
	file, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(normalized), "")
	if err != nil {
		return false, err
	}

	found := false
	syntax.Walk(file, func(node syntax.Node) bool {
		if found {
			return false
		}
		call, ok := node.(*syntax.CallExpr)
		if !ok || len(call.Args) == 0 {
			return true
		}
		executable, expandErr := expand.Literal(nil, call.Args[0])
		if expandErr == nil && path.Base(executable) == command {
			found = true
		}
		return true
	})
	return found, nil
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
		invokesAPK, err := nativeTestInvokes(step.Runs, "apk")
		require.NoError(t, err, name)
		if invokesAPK || step.Uses == "test/virtualpackage" || step.Uses == "test/emptypackage" {
			assert.Contains(t, test.Environment.Contents.Packages, "apk-tools", name)
		}
		invokesWget, err := nativeTestInvokes(step.Runs, "wget")
		require.NoError(t, err, name)
		if invokesWget {
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
