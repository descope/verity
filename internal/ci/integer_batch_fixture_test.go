package ci

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func integerBatchFixturePlan() IntegerBatchPlan {
	return IntegerBatchPlan{
		SchemaVersion: IntegerBatchSchemaVersion,
		SourceSHA:     testSourceSHA,
		RunID:         42,
		RunAttempt:    3,
		PublicationID: "integer-publication-42-3",
		BatchID:       "42-3",
		Mode:          IntegerBatchModeSnapshot,
		Event:         IntegerBatchEventSchedule,
		Targets: []IntegerBatchTarget{
			{Name: "alpha", Version: "1", Type: "default", ArtifactKey: "alpha-1-default-000000000001", Shard: "1", ExpectedPackages: []string{"alpha"}, PublishPackages: []string{"alpha"}},
			{Name: "beta", Version: "1", Type: "default", ArtifactKey: "beta-1-default-000000000002", Shard: "2", ExpectedPackages: []string{"beta"}, PublishPackages: []string{"beta"}},
		},
		Packages: []IntegerPlannedPackage{
			{Architecture: IntegerArchitectureX8664, Name: "alpha", Producer: "alpha:1-default"},
			{Architecture: IntegerArchitectureAArch64, Name: "alpha", Producer: "alpha:1-default"},
			{Architecture: IntegerArchitectureX8664, Name: "beta", Producer: "beta:1-default"},
			{Architecture: IntegerArchitectureAArch64, Name: "beta", Producer: "beta:1-default"},
		},
	}
}

func writeIntegerTestAPK(t *testing.T, path, name, architecture, payload string) {
	t.Helper()
	var apk bytes.Buffer
	pkgInfo := fmt.Sprintf("pkgname = %s\npkgver = 1.0.0-r0\narch = %s\nsize = %d\n", name, architecture, len(payload))
	apk.Write(integerTestTarGzip(t, map[string]string{".PKGINFO": pkgInfo}))
	apk.Write(integerTestTarGzip(t, map[string]string{"usr/bin/" + name: payload}))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, apk.Bytes(), 0o600))
}

func integerTestTarGzip(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	tarWriter := tar.NewWriter(gzipWriter)
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		contents := entries[name]
		require.NoError(t, tarWriter.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(contents))}))
		_, err := tarWriter.Write([]byte(contents))
		require.NoError(t, err)
	}
	require.NoError(t, tarWriter.Close())
	require.NoError(t, gzipWriter.Close())
	return buffer.Bytes()
}

func stageIntegerFixtureComponent(t *testing.T, plan *IntegerBatchPlan, targetID, packageName string) string {
	t.Helper()
	packages := t.TempDir()
	writeIntegerTestAPK(t, filepath.Join(packages, "x86_64", packageName+".apk"), packageName, "x86_64", targetID+"-x86")
	writeIntegerTestAPK(t, filepath.Join(packages, "aarch64", packageName+".apk"), packageName, "aarch64", targetID+"-arm")
	output := t.TempDir()
	_, err := StageIntegerComponent(t.Context(), &IntegerComponentOptions{
		Plan: plan, TargetID: targetID, PackagesDir: packages, OutputDir: output,
	})
	require.NoError(t, err)
	return output
}

func mutateIntegerComponentManifest(t *testing.T, root string, mutate func(*IntegerComponentManifest)) {
	t.Helper()
	path := filepath.Join(root, IntegerComponentManifestName)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	manifest, err := ParseIntegerComponentManifest(data)
	require.NoError(t, err)
	mutate(&manifest)
	data, err = MarshalIntegerComponentManifest(&manifest)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o600))
}

func copyIntegerComponentDir(t *testing.T, source string) string {
	t.Helper()
	destination := t.TempDir()
	sourceDirectory, err := os.OpenRoot(source)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sourceDirectory.Close()) })
	err = filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := sourceDirectory.ReadFile(relative)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o600)
	})
	require.NoError(t, err)
	return destination
}

func integerPackageFileIDs(packages []IntegerPackageFile) []string {
	ids := make([]string, 0, len(packages))
	for _, pkg := range packages {
		ids = append(ids, string(pkg.Architecture)+"/"+pkg.Name)
	}
	slices.Sort(ids)
	return ids
}

func integerPublishedPackageIDs(packages []IntegerPublishedPackage) []string {
	ids := make([]string, 0, len(packages))
	for _, pkg := range packages {
		ids = append(ids, string(pkg.Architecture)+"/"+pkg.Name)
	}
	slices.Sort(ids)
	return ids
}

func integerArtifactNames(shards []IntegerShardManifest) []string {
	names := make([]string, 0, len(shards))
	for index := range shards {
		names = append(names, shards[index].Artifact.Name)
	}
	slices.Sort(names)
	return names
}

func testArtifactDigest(seed string) string {
	return "sha256:" + strings.Repeat(seed, 64)
}
