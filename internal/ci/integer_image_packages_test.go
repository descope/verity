package ci

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTestIntegerPackages_runsEveryStagedRecipeNatively(t *testing.T) {
	// Given: two staged recipes, a shared pipeline directory, and an aarch64 runner.
	root := t.TempDir()
	writeIntegerPackageSpec(t, root, "rclone", "rclone")
	writeIntegerPackageSpec(t, root, "step-ca", "step-ca")
	require.NoError(t, os.MkdirAll(filepath.Join(root, "melange-work", "pipelines"), 0o755))
	runner := &recordingIntegerImageRunner{}

	// When: Go-owned native package testing runs.
	err := TestIntegerPackages(t.Context(), &IntegerPackageTestOptions{
		Architecture: IntegerArchitectureAArch64, Workspace: root, Timeout: 30 * time.Minute, Runner: runner,
	})

	// Then: each recipe is tested once with the exact local repository, key,
	// architecture, and pipeline inputs.
	require.NoError(t, err)
	assert.Equal(t, []string{"melange:test", "melange:test"}, runner.callIDs())
	for _, call := range runner.calls {
		assert.Contains(t, call.Args, "aarch64")
		assert.Contains(t, call.Args, filepath.Join(root, "packages", "repo"))
		assert.Contains(t, call.Args, filepath.Join(root, "packages", "repo", "melange-aarch64.rsa.pub"))
		assert.Contains(t, call.Args, "melange-work/pipelines")
		testPackage := slices.Index(call.Args, "--test-package-append")
		require.GreaterOrEqual(t, testPackage, 0)
		require.Less(t, testPackage+1, len(call.Args))
		assert.Equal(t, "busybox", call.Args[testPackage+1])
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

func writeIntegerPackageSpec(t *testing.T, root, directory, packageName string) {
	t.Helper()
	path := filepath.Join(root, "melange-work", "specs", directory, "build.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("package:\n  name: "+packageName+"\n"), 0o600))
}
