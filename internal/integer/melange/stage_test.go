package melange

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStageLockedInputs(t *testing.T) {
	root := t.TempDir()
	recipe := "package:\n  name: cilium-1.19\n"
	asset := "patch"
	pipeline := "name: test\n"
	writeTestFile(t, testPath(root, "packages/bespoke/locked/cilium-1.19.yaml"), recipe)
	writeTestFile(t, testPath(root, "packages/bespoke/locked/cilium-1.19/fix.patch"), asset)
	writeTestFile(t, testPath(root, "packages/pipelines/test/check.yaml"), pipeline)
	writeTestFile(t, testPath(root, "packages/upstream.lock.json"), fmt.Sprintf(`{
  "packages":{"cilium-1.19":{"file":"cilium-1.19.yaml","sha256":"%s","assets":{"cilium-1.19/fix.patch":"%s"}}},
  "pipeline_files":{"test/check.yaml":"%s"}
}`, testSHA(recipe), testSHA(asset), testSHA(pipeline)))

	err := Stage(testPathsPtr(root), Spec{Upstream: "cilium-1.19"})
	require.NoError(t, err)

	stagedRecipe, err := os.ReadFile(testPath(root, "melange-work/specs/cilium-1.19/build.yaml"))
	require.NoError(t, err)
	assert.Equal(t, recipe, string(stagedRecipe))
	stagedAsset, err := os.ReadFile(testPath(root, "melange-work/specs/cilium-1.19/fix.patch"))
	require.NoError(t, err)
	assert.Equal(t, asset, string(stagedAsset))
	stagedPipeline, err := os.ReadFile(testPath(root, "melange-work/pipelines/test/check.yaml"))
	require.NoError(t, err)
	assert.Equal(t, pipeline, string(stagedPipeline))
}

func TestStageRejectsTamperedLockedAsset(t *testing.T) {
	root := t.TempDir()
	recipe := "package:\n  name: cilium-1.19\n"
	writeTestFile(t, testPath(root, "packages/bespoke/locked/cilium-1.19.yaml"), recipe)
	writeTestFile(t, testPath(root, "packages/bespoke/locked/cilium-1.19/fix.patch"), "tampered")
	writeTestFile(t, testPath(root, "packages/upstream.lock.json"), fmt.Sprintf(`{
  "packages":{"cilium-1.19":{"file":"cilium-1.19.yaml","sha256":"%s","assets":{"cilium-1.19/fix.patch":"%s"}}},
  "pipeline_files":{}
}`, testSHA(recipe), testSHA("expected")))

	err := Stage(testPathsPtr(root), Spec{Upstream: "cilium-1.19"})
	require.Error(t, err)
	assert.ErrorContains(t, err, "sha256 mismatch")
}

func TestStageRejectsUntrackedPipeline(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, testPath(root, "packages/bespoke/custom.yaml"), "package:\n  name: custom\n")
	writeTestFile(t, testPath(root, "packages/pipelines/test/untracked.yaml"), "name: untracked\n")
	writeTestFile(t, testPath(root, "packages/upstream.lock.json"), `{"packages":{},"pipeline_files":{}}`)

	err := Stage(testPathsPtr(root), Spec{Bespoke: []string{"custom.yaml"}})
	require.Error(t, err)
	assert.ErrorContains(t, err, "pipeline file set does not match lock manifest")
}

func TestStageRejectsSymlinkedLockedRoot(t *testing.T) {
	root := t.TempDir()
	external := filepath.Join(root, "external")
	recipe := "package:\n  name: cilium-1.19\n"
	writeTestFile(t, filepath.Join(external, "cilium-1.19.yaml"), recipe)
	require.NoError(t, os.MkdirAll(testPath(root, "packages/bespoke"), 0o755))
	require.NoError(t, os.Symlink(external, testPath(root, "packages/bespoke/locked")))
	writeTestFile(t, testPath(root, "packages/upstream.lock.json"), fmt.Sprintf(`{
  "packages":{"cilium-1.19":{"file":"cilium-1.19.yaml","sha256":"%s","assets":{}}},
  "pipeline_files":{}
}`, testSHA(recipe)))

	err := Stage(testPathsPtr(root), Spec{Upstream: "cilium-1.19"})
	require.Error(t, err)
	assert.ErrorContains(t, err, "symlink")
}
