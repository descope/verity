package melange

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStageRejectsUnsafeBespokePath(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, testPath(root, "packages/upstream.lock.json"), `{"packages":{},"pipeline_files":{}}`)

	err := Stage(testPathsPtr(root), Spec{Bespoke: []string{"../evil.yaml"}})

	require.Error(t, err)
	assert.ErrorContains(t, err, "unsafe relative path")
}

func TestStageRejectsMissingBespokeRecipe(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, testPath(root, "packages/upstream.lock.json"), `{"packages":{},"pipeline_files":{}}`)
	writeTestFile(t, testPath(root, "packages/bespoke/.keep"), "")

	err := Stage(testPathsPtr(root), Spec{Bespoke: []string{"custom.yaml"}})

	require.Error(t, err)
	assert.ErrorContains(t, err, "custom.yaml")
}

func TestStageBuildsBespokeRecipeWithoutLockedBaseline(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, testPath(root, "packages/upstream.lock.json"), `{"packages":{},"pipeline_files":{}}`)
	writeTestFile(t, testPath(root, "packages/bespoke/custom.yaml"), "package:\n  name: custom\n")

	err := Stage(testPathsPtr(root), Spec{Bespoke: []string{"custom.yaml"}})

	require.NoError(t, err)
	data, readErr := os.ReadFile(testPath(root, "melange-work/specs/custom.yaml/build.yaml"))
	require.NoError(t, readErr)
	assert.Equal(t, "package:\n  name: custom\n", string(data))
}

func TestStageRejectsUnsafeLockedRecipePath(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, testPath(root, "packages/bespoke/locked/.keep"), "")
	writeTestFile(t, testPath(root, "packages/upstream.lock.json"), `{
  "packages":{"evil":{"file":"../evil.yaml","sha256":"abc","assets":{}}},
  "pipeline_files":{}
}`)

	err := Stage(testPathsPtr(root), Spec{Upstream: "evil"})

	require.Error(t, err)
	assert.ErrorContains(t, err, "unsafe relative path")
}

func TestStageRejectsUnlistedLockedAsset(t *testing.T) {
	root := t.TempDir()
	recipe := "package:\n  name: cilium-1.19\n"
	writeTestFile(t, testPath(root, "packages/bespoke/locked/cilium-1.19.yaml"), recipe)
	writeTestFile(t, testPath(root, "packages/bespoke/locked/cilium-1.19/unlisted.patch"), "patch")
	writeTestFile(t, testPath(root, "packages/upstream.lock.json"), fmt.Sprintf(`{
  "packages":{"cilium-1.19":{"file":"cilium-1.19.yaml","sha256":"%s","assets":{}}},
  "pipeline_files":{}
}`, testSHA(recipe)))

	err := Stage(testPathsPtr(root), Spec{Upstream: "cilium-1.19"})

	require.Error(t, err)
	assert.ErrorContains(t, err, "sidecar file set does not match lock manifest")
}

func TestStageRejectsSymlinkedLockedAsset(t *testing.T) {
	root := t.TempDir()
	recipe := "package:\n  name: cilium-1.19\n"
	asset := "patch"
	writeTestFile(t, testPath(root, "packages/bespoke/locked/cilium-1.19.yaml"), recipe)
	writeTestFile(t, testPath(root, "external/fix.patch"), asset)
	require.NoError(t, os.MkdirAll(testPath(root, "packages/bespoke/locked/cilium-1.19"), 0o755))
	require.NoError(t, os.Symlink(testPath(root, "external/fix.patch"), testPath(root, "packages/bespoke/locked/cilium-1.19/fix.patch")))
	writeTestFile(t, testPath(root, "packages/upstream.lock.json"), fmt.Sprintf(`{
  "packages":{"cilium-1.19":{"file":"cilium-1.19.yaml","sha256":"%s","assets":{"cilium-1.19/fix.patch":"%s"}}},
  "pipeline_files":{}
}`, testSHA(recipe), testSHA(asset)))

	err := Stage(testPathsPtr(root), Spec{Upstream: "cilium-1.19"})

	require.Error(t, err)
	assert.ErrorContains(t, err, "not a regular file or directory")
}

func TestStageRejectsSymlinkedSidecarRoot(t *testing.T) {
	root := t.TempDir()
	recipe := "package:\n  name: cilium-1.19\n"
	writeTestFile(t, testPath(root, "packages/bespoke/locked/cilium-1.19.yaml"), recipe)
	require.NoError(t, os.MkdirAll(testPath(root, "external/sidecar"), 0o755))
	require.NoError(t, os.Symlink(testPath(root, "external/sidecar"), testPath(root, "packages/bespoke/locked/cilium-1.19")))
	writeTestFile(t, testPath(root, "packages/upstream.lock.json"), fmt.Sprintf(`{
  "packages":{"cilium-1.19":{"file":"cilium-1.19.yaml","sha256":"%s","assets":{}}},
  "pipeline_files":{}
}`, testSHA(recipe)))

	err := Stage(testPathsPtr(root), Spec{Upstream: "cilium-1.19"})

	require.Error(t, err)
	assert.ErrorContains(t, err, "contains a symlink")
}

func TestStageRejectsSpecialFileSidecarRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("named pipes require Unix")
	}
	root := t.TempDir()
	recipe := "package:\n  name: cilium-1.19\n"
	writeTestFile(t, testPath(root, "packages/bespoke/locked/cilium-1.19.yaml"), recipe)
	require.NoError(t, syscall.Mkfifo(testPath(root, "packages/bespoke/locked/cilium-1.19"), 0o600))
	writeTestFile(t, testPath(root, "packages/upstream.lock.json"), fmt.Sprintf(`{
  "packages":{"cilium-1.19":{"file":"cilium-1.19.yaml","sha256":"%s","assets":{}}},
  "pipeline_files":{}
}`, testSHA(recipe)))

	err := Stage(testPathsPtr(root), Spec{Upstream: "cilium-1.19"})

	require.Error(t, err)
	assert.ErrorContains(t, err, "is not a real directory")
}

func TestStageRejectsSymlinkedPipelineRoot(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, testPath(root, "packages/upstream.lock.json"), `{"packages":{},"pipeline_files":{}}`)
	writeTestFile(t, testPath(root, "packages/bespoke/custom.yaml"), "package:\n  name: custom\n")
	require.NoError(t, os.MkdirAll(testPath(root, "external/pipelines"), 0o755))
	require.NoError(t, os.Symlink(testPath(root, "external/pipelines"), testPath(root, "packages/pipelines")))

	err := Stage(testPathsPtr(root), Spec{Bespoke: []string{"custom.yaml"}})

	require.Error(t, err)
	assert.ErrorContains(t, err, "symlink")
}

func TestStageCleanupDoesNotFollowSwappedWorkDirectory(t *testing.T) {
	root := t.TempDir()
	paths := testPaths(root)
	writeTestFile(t, testPath(root, "packages/bespoke/custom.yaml"), "package:\n  name: custom\n")
	writeTestFile(t, testPath(root, "packages/upstream.lock.json"), `{"packages":{},"pipeline_files":{}}`)
	external := t.TempDir()
	sentinel := filepath.Join(external, "specs", "sentinel")
	writeTestFile(t, sentinel, "keep")

	stageAfterManagedRoot = func() {
		require.NoError(t, os.Rename(paths.WorkDir, paths.WorkDir+".validated"))
		require.NoError(t, os.Symlink(external, paths.WorkDir))
	}
	t.Cleanup(func() { stageAfterManagedRoot = nil })

	err := Stage(&paths, Spec{Bespoke: []string{"custom.yaml"}})

	require.Error(t, err)
	assert.FileExists(t, sentinel)
}
