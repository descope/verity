package melange

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArtifactsExistRequiresMatchingSpecAndRegularFiles(t *testing.T) {
	root := t.TempDir()
	paths := testPaths(root)
	arch := ArchitectureAArch64
	spec := Spec{Upstream: "cilium-1.19", EnvFile: "fips.env"}
	recipe := "package:\n  name: cilium-1.19\n"
	writeInputs := func(root string) {
		writeTestFile(t, testPath(root, "packages/bespoke/locked/cilium-1.19.yaml"), recipe)
		writeTestFile(t, testPath(root, "packages/upstream.lock.json"), fmt.Sprintf(`{
  "packages":{"cilium-1.19":{"file":"cilium-1.19.yaml","sha256":"%s","assets":{}}},
  "pipeline_files":{}
}`, testSHA(recipe)))
		writeTestFile(t, testPath(root, "packages/overrides/fips.env"), "GOFIPS140=v1.0.0\n")
	}
	writeInputs(root)
	writeTestFile(t, filepath.Join(paths.RepoDir, string(arch), "APKINDEX.tar.gz"), "index")
	writeTestFile(t, filepath.Join(paths.RepoDir, "melange-"+string(arch)+".rsa.pub"), "public")
	require.NoError(t, writeArtifactMarker(&paths, spec, arch))

	assert.True(t, ArtifactsExist(&paths, spec, arch))
	assert.False(t, ArtifactsExist(&paths, Spec{Upstream: "caddy"}, arch))

	targets := []struct {
		name string
		path func(*Paths) string
	}{
		{name: "index", path: func(paths *Paths) string {
			return filepath.Join(paths.RepoDir, string(arch), "APKINDEX.tar.gz")
		}},
		{name: "public key", path: func(paths *Paths) string {
			return filepath.Join(paths.RepoDir, "melange-"+string(arch)+".rsa.pub")
		}},
		{name: "marker", path: func(paths *Paths) string {
			return artifactMarkerPath(paths, arch)
		}},
	}
	for _, target := range targets {
		t.Run(target.name, func(t *testing.T) {
			symlinkRoot := t.TempDir()
			symlinkPaths := testPaths(symlinkRoot)
			writeInputs(symlinkRoot)
			writeTestFile(t, filepath.Join(symlinkPaths.RepoDir, string(arch), "APKINDEX.tar.gz"), "index")
			writeTestFile(t, filepath.Join(symlinkPaths.RepoDir, "melange-"+string(arch)+".rsa.pub"), "public")
			require.NoError(t, writeArtifactMarker(&symlinkPaths, spec, arch))
			path := target.path(&symlinkPaths)
			require.NoError(t, os.Remove(path))
			external := filepath.Join(symlinkRoot, "external")
			writeTestFile(t, external, "replacement")
			require.NoError(t, os.Symlink(external, path))
			assert.False(t, ArtifactsExist(&symlinkPaths, spec, arch))
		})
	}
}

func TestArtifactsExistInvalidatesChangedBuildInputs(t *testing.T) {
	root := t.TempDir()
	paths := testPaths(root)
	arch := ArchitectureX8664
	recipe := "package:\n  name: caddy\n"
	lock := fmt.Sprintf(`{
  "packages":{"caddy":{"file":"caddy.yaml","sha256":"%s","assets":{}}},
  "pipeline_files":{}
}`, testSHA(recipe))
	spec := Spec{Upstream: "caddy", EnvFile: "fips.env"}

	writeTestFile(t, testPath(root, "packages/bespoke/locked/caddy.yaml"), recipe)
	writeTestFile(t, testPath(root, "packages/upstream.lock.json"), lock)
	writeTestFile(t, testPath(root, "packages/overrides/fips.env"), "GOFIPS140=v1.0.0\n")
	writeTestFile(t, filepath.Join(paths.RepoDir, string(arch), "APKINDEX.tar.gz"), "index")
	writeTestFile(t, filepath.Join(paths.RepoDir, "melange-"+string(arch)+".rsa.pub"), "public")
	require.NoError(t, writeArtifactMarker(&paths, spec, arch))
	require.True(t, ArtifactsExist(&paths, spec, arch))

	writeTestFile(t, testPath(root, "packages/overrides/fips.env"), "GOFIPS140=v1.0.1\n")
	assert.False(t, ArtifactsExist(&paths, spec, arch))

	writeTestFile(t, testPath(root, "packages/overrides/fips.env"), "GOFIPS140=v1.0.0\n")
	writeTestFile(t, testPath(root, "packages/bespoke/locked/caddy.yaml"), recipe+"  epoch: 1\n")
	assert.False(t, ArtifactsExist(&paths, spec, arch))
}

func TestArtifactsExistInvalidatesChangedBespokeRecipe(t *testing.T) {
	// Given: a signed package repository built from a direct bespoke recipe.
	root := t.TempDir()
	paths := testPaths(root)
	arch := ArchitectureX8664
	spec := Spec{Bespoke: []string{"custom.yaml"}}
	writeTestFile(t, testPath(root, "packages/bespoke/custom.yaml"), "package:\n  name: custom\n")
	writeTestFile(t, testPath(root, "packages/upstream.lock.json"), `{"packages":{},"pipeline_files":{}}`)
	writeTestFile(t, filepath.Join(paths.RepoDir, string(arch), "APKINDEX.tar.gz"), "index")
	writeTestFile(t, filepath.Join(paths.RepoDir, "melange-"+string(arch)+".rsa.pub"), "public")
	require.NoError(t, writeArtifactMarker(&paths, spec, arch))
	require.True(t, ArtifactsExist(&paths, spec, arch))

	// When: the bespoke recipe content changes without rebuilding.
	writeTestFile(t, testPath(root, "packages/bespoke/custom.yaml"), "package:\n  name: custom\n  epoch: 1\n")

	// Then: the cached artifact fingerprint is rejected.
	assert.False(t, ArtifactsExist(&paths, spec, arch))
}

func TestArtifactsExistRejectsSymlinkedRootsAndChangedOutputs(t *testing.T) {
	setup := func(t *testing.T) (string, Paths, Spec, Architecture) {
		t.Helper()
		root := t.TempDir()
		paths := testPaths(root)
		recipe := "package:\n  name: caddy\n"
		spec := Spec{Upstream: "caddy"}
		arch := ArchitectureX8664
		writeTestFile(t, testPath(root, "packages/bespoke/locked/caddy.yaml"), recipe)
		writeTestFile(t, testPath(root, "packages/upstream.lock.json"), fmt.Sprintf(`{
  "packages":{"caddy":{"file":"caddy.yaml","sha256":"%s","assets":{}}},
  "pipeline_files":{}
}`, testSHA(recipe)))
		writeTestFile(t, filepath.Join(paths.RepoDir, string(arch), "APKINDEX.tar.gz"), "index")
		writeTestFile(t, filepath.Join(paths.RepoDir, string(arch), "caddy.apk"), "package")
		writeTestFile(t, filepath.Join(paths.RepoDir, "melange-"+string(arch)+".rsa.pub"), "public")
		require.NoError(t, writeArtifactMarker(&paths, spec, arch))
		require.True(t, ArtifactsExist(&paths, spec, arch))
		return root, paths, spec, arch
	}

	t.Run("changed output", func(t *testing.T) {
		_, paths, spec, arch := setup(t)
		writeTestFile(t, filepath.Join(paths.RepoDir, string(arch), "caddy.apk"), "replaced")
		assert.False(t, ArtifactsExist(&paths, spec, arch))
	})

	t.Run("repository root", func(t *testing.T) {
		root, paths, spec, arch := setup(t)
		backing := testPath(root, "external/repository")
		require.NoError(t, os.MkdirAll(filepath.Dir(backing), 0o755))
		require.NoError(t, os.Rename(paths.RepoDir, backing))
		require.NoError(t, os.Symlink(backing, paths.RepoDir))
		assert.False(t, ArtifactsExist(&paths, spec, arch))
	})

	t.Run("repository ancestor", func(t *testing.T) {
		root, paths, spec, arch := setup(t)
		backing := testPath(root, "external/packages")
		require.NoError(t, os.MkdirAll(filepath.Dir(backing), 0o755))
		require.NoError(t, os.Rename(testPath(root, "packages"), backing))
		require.NoError(t, os.Symlink(backing, testPath(root, "packages")))
		assert.False(t, ArtifactsExist(&paths, spec, arch))
	})
}
