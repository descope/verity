package ci

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestTestIntegerPackages_runsEveryStagedRecipeNatively(t *testing.T) {
	// Given: two staged recipes, a shared pipeline directory, and an aarch64 runner.
	root := t.TempDir()
	writeIntegerPackageSpec(t, root, "rclone", "rclone")
	writeIntegerPackageSpec(t, root, "step-ca", "step-ca")
	index := "P:rclone\nV:1.67.0-r1\n\nP:step-ca\nV:0.28.3-r2\n"
	indexPath := filepath.Join(root, "packages", "repo", "aarch64", "APKINDEX.tar.gz")
	require.NoError(t, os.MkdirAll(filepath.Dir(indexPath), 0o755))
	require.NoError(t, os.WriteFile(indexPath, integerTestTarGzip(t, map[string]string{"APKINDEX": index}), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "melange-work", "pipelines"), 0o755))
	runner := &recordingIntegerImageRunner{}

	// When: Go-owned native package testing runs.
	err := TestIntegerPackages(t.Context(), &IntegerPackageTestOptions{
		Architecture: IntegerArchitectureAArch64, Workspace: root, Timeout: 30 * time.Minute, Runner: runner,
	})

	// Then: each recipe is tested once with the exact local package, repository,
	// key, architecture, and pipeline inputs.
	require.NoError(t, err)
	assert.Equal(t, []string{"melange:test", "melange:test"}, runner.callIDs())
	expectedPins := map[string]string{"rclone": "rclone=1.67.0-r1", "step-ca": "step-ca=0.28.3-r2"}
	for _, call := range runner.calls {
		assert.Contains(t, call.Args, "aarch64")
		assert.Contains(t, call.Args, filepath.Join(root, "packages", "repo"))
		assert.Contains(t, call.Args, filepath.Join(root, "packages", "repo", "melange-aarch64.rsa.pub"))
		assert.Contains(t, call.Args, "melange-work/pipelines")
		var testPackages []string
		for index, argument := range call.Args {
			if argument == "--test-package-append" {
				require.Less(t, index+1, len(call.Args))
				testPackages = append(testPackages, call.Args[index+1])
			}
		}
		packageName := call.Args[len(call.Args)-1]
		assert.Equal(t, []string{"busybox"}, testPackages)
		testSpec, readErr := os.ReadFile(filepath.Join(root, "melange-work", "specs", packageName, "test.yaml"))
		require.NoError(t, readErr)
		var document struct {
			Test struct {
				Environment struct {
					Contents struct {
						Packages []string `yaml:"packages"`
					} `yaml:"contents"`
				} `yaml:"environment"`
			} `yaml:"test"`
		}
		require.NoError(t, yaml.Unmarshal(testSpec, &document))
		assert.Contains(t, document.Test.Environment.Contents.Packages, expectedPins[packageName])
	}
}

func TestTestIntegerPackages_rejectsUnsupportedArchitecture_beforeCommands(t *testing.T) {
	// Given: one staged recipe and an unsupported architecture.
	root := t.TempDir()
	writeIntegerPackageSpec(t, root, "alpha", "alpha")
	runner := &recordingIntegerImageRunner{}

	// When: package testing parses the architecture boundary.
	err := TestIntegerPackages(t.Context(), &IntegerPackageTestOptions{
		Architecture: "armv7", Workspace: root, Timeout: time.Minute, Runner: runner,
	})

	// Then: no external test command is started.
	require.ErrorIs(t, err, ErrIntegerBatchPlan)
	assert.Empty(t, runner.calls)
}

func TestPinIntegerPackageTestSpec_pinsMainAndSubpackageIndependently(t *testing.T) {
	// Given: a staged recipe with separate main and subpackage tests.
	spec := []byte(`package:
  name: crane
  version: 0.21.7
test:
  pipeline:
    - runs: crane version
subpackages:
  - name: ${{package.name}}-cov
    test:
      pipeline:
        - runs: crane version
`)
	versions := map[string]string{"crane": "0.21.7-r1", "crane-cov": "0.21.7-r1"}

	// When: the test-only recipe is bound to locally built APK identities.
	pinned, err := pinIntegerPackageTestSpec(spec, versions)

	// Then: each isolated test environment pins only its own package under test.
	require.NoError(t, err)
	var document struct {
		Test struct {
			Environment struct {
				Contents struct {
					Packages []string `yaml:"packages"`
				} `yaml:"contents"`
			} `yaml:"environment"`
		} `yaml:"test"`
		Subpackages []struct {
			Test struct {
				Environment struct {
					Contents struct {
						Packages []string `yaml:"packages"`
					} `yaml:"contents"`
				} `yaml:"environment"`
			} `yaml:"test"`
		} `yaml:"subpackages"`
	}
	require.NoError(t, yaml.Unmarshal(pinned, &document))
	assert.Equal(t, []string{"crane=0.21.7-r1"}, document.Test.Environment.Contents.Packages)
	require.Len(t, document.Subpackages, 1)
	assert.Equal(t, []string{"crane-cov=0.21.7-r1"}, document.Subpackages[0].Test.Environment.Contents.Packages)
}

func writeIntegerPackageSpec(t *testing.T, root, directory, packageName string) {
	t.Helper()
	path := filepath.Join(root, "melange-work", "specs", directory, "build.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	recipe := "package:\n  name: " + packageName + "\n  version: 1.0.0\ntest:\n  pipeline:\n    - runs: true\n"
	require.NoError(t, os.WriteFile(path, []byte(recipe), 0o600))
}
