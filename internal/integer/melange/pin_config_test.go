package melange

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestPinConfigPackagesPinsLocalVersionsAcrossArchitectures(t *testing.T) {
	// Given: both local repositories contain the same rebuilt package revisions.
	root := t.TempDir()
	configPath := filepath.Join(root, "config.apko.yaml")
	repositoryDir := filepath.Join(root, "repo")
	writeTestFile(t, configPath, `contents:
  repositories:
    - https://packages.example.test/os
  packages:
    - crane
    - linkerd2-cli=25.12.3-r99
    - bash
`)
	index := "P:crane\nV:0.21.7-r1\n\nP:linkerd2-cli\nV:25.12.3-r100\n\n"
	writeAPKIndexArchive(t, filepath.Join(repositoryDir, "x86_64", "APKINDEX.tar.gz"), index)
	writeAPKIndexArchive(t, filepath.Join(repositoryDir, "aarch64", "APKINDEX.tar.gz"), index)

	// When: the publish config is pinned against both local indexes.
	err := PinConfigPackages(PinConfigOptions{
		RootDir:       root,
		ConfigPath:    configPath,
		RepositoryDir: repositoryDir,
		Architectures: []Architecture{ArchitectureX8664, ArchitectureAArch64},
	})

	// Then: local packages select the tagged repository and exact revision.
	require.NoError(t, err)
	assert.Equal(t, []string{
		"crane=0.21.7-r1@local",
		"linkerd2-cli=25.12.3-r100@local",
		"bash",
	}, readConfigPackages(t, configPath))
}

func TestPinConfigPackagesPinsLocalDependencyClosure(t *testing.T) {
	// Given: the configured package depends on local subpackages in both architecture indexes.
	root := t.TempDir()
	configPath := filepath.Join(root, "config.apko.yaml")
	repositoryDir := filepath.Join(root, "repo")
	writeTestFile(t, configPath, "contents:\n  packages: [postgresql-15, bash]\n")
	index := "P:postgresql-15\nV:15.14-r0\nD:postgresql-15-base=15.14-r0 libpq-15=15.14-r0 tzdata\n\n" +
		"P:postgresql-15-base\nV:15.14-r0\nD:libpq-15=15.14-r0\n\n" +
		"P:libpq-15\nV:15.14-r0\n\n"
	writeAPKIndexArchive(t, filepath.Join(repositoryDir, "x86_64", "APKINDEX.tar.gz"), index)
	writeAPKIndexArchive(t, filepath.Join(repositoryDir, "aarch64", "APKINDEX.tar.gz"), index)

	// When: the publish config is pinned against both local indexes.
	err := PinConfigPackages(PinConfigOptions{
		RootDir:       root,
		ConfigPath:    configPath,
		RepositoryDir: repositoryDir,
		Architectures: []Architecture{ArchitectureX8664, ArchitectureAArch64},
	})

	// Then: the root and every local dependency select the tagged repository exactly once.
	require.NoError(t, err)
	assert.Equal(t, []string{
		"postgresql-15=15.14-r0@local",
		"bash",
		"postgresql-15-base=15.14-r0@local",
		"libpq-15=15.14-r0@local",
	}, readConfigPackages(t, configPath))
}

func TestPinConfigPackagesRejectsArchitectureVersionMismatch(t *testing.T) {
	// Given: the local repositories disagree on the package revision.
	root := t.TempDir()
	configPath := filepath.Join(root, "config.apko.yaml")
	repositoryDir := filepath.Join(root, "repo")
	writeTestFile(t, configPath, "contents:\n  packages: [crane]\n")
	writeAPKIndexArchive(t, filepath.Join(repositoryDir, "x86_64", "APKINDEX.tar.gz"), "P:crane\nV:0.21.7-r1\n\n")
	writeAPKIndexArchive(t, filepath.Join(repositoryDir, "aarch64", "APKINDEX.tar.gz"), "P:crane\nV:0.21.7-r2\n\n")

	// When: the multi-architecture config is pinned.
	err := PinConfigPackages(PinConfigOptions{
		RootDir:       root,
		ConfigPath:    configPath,
		RepositoryDir: repositoryDir,
		Architectures: []Architecture{ArchitectureX8664, ArchitectureAArch64},
	})

	// Then: publishing stops instead of selecting different builds per architecture.
	require.ErrorIs(t, err, errPinnedPackageVersionConflict)
}

func TestPinConfigPackagesRejectsConfigWithoutLocalPackage(t *testing.T) {
	// Given: the downloaded repository does not provide any configured package.
	root := t.TempDir()
	configPath := filepath.Join(root, "config.apko.yaml")
	repositoryDir := filepath.Join(root, "repo")
	writeTestFile(t, configPath, "contents:\n  packages: [bash]\n")
	index := "P:crane\nV:0.21.7-r1\n\n"
	writeAPKIndexArchive(t, filepath.Join(repositoryDir, "x86_64", "APKINDEX.tar.gz"), index)
	writeAPKIndexArchive(t, filepath.Join(repositoryDir, "aarch64", "APKINDEX.tar.gz"), index)

	// When: the publish config is pinned.
	err := PinConfigPackages(PinConfigOptions{
		RootDir:       root,
		ConfigPath:    configPath,
		RepositoryDir: repositoryDir,
		Architectures: []Architecture{ArchitectureX8664, ArchitectureAArch64},
	})

	// Then: publishing fails closed because the local artifact would be unused.
	require.ErrorIs(t, err, errPinnedPackageNotUsed)
}

func TestPinConfigPackagesRejectsNonScalarPackageEntry(t *testing.T) {
	// Given: the apko package list contains a YAML mapping instead of a package string.
	root := t.TempDir()
	configPath := filepath.Join(root, "config.apko.yaml")
	repositoryDir := filepath.Join(root, "repo")
	writeTestFile(t, configPath, "contents:\n  packages:\n    - name: crane\n")
	index := "P:crane\nV:0.21.7-r1\n\n"
	writeAPKIndexArchive(t, filepath.Join(repositoryDir, "x86_64", "APKINDEX.tar.gz"), index)
	writeAPKIndexArchive(t, filepath.Join(repositoryDir, "aarch64", "APKINDEX.tar.gz"), index)

	// When: the malformed publish config is pinned.
	err := PinConfigPackages(PinConfigOptions{
		RootDir:       root,
		ConfigPath:    configPath,
		RepositoryDir: repositoryDir,
		Architectures: []Architecture{ArchitectureX8664, ArchitectureAArch64},
	})

	// Then: the parser returns a stable boundary error.
	require.ErrorIs(t, err, errPinnedConfigPackageNotScalar)
}

func TestPinConfigPackagesRejectsSymlinkConfig(t *testing.T) {
	// Given: the requested config path is a symlink to another writable file.
	root := t.TempDir()
	configPath := filepath.Join(root, "config.apko.yaml")
	victim := filepath.Join(root, "victim.apko.yaml")
	repositoryDir := filepath.Join(root, "repo")
	writeTestFile(t, victim, "contents:\n  packages: [crane]\n")
	require.NoError(t, os.Symlink(victim, configPath))
	index := "P:crane\nV:0.21.7-r1\n\n"
	writeAPKIndexArchive(t, filepath.Join(repositoryDir, "x86_64", "APKINDEX.tar.gz"), index)
	writeAPKIndexArchive(t, filepath.Join(repositoryDir, "aarch64", "APKINDEX.tar.gz"), index)

	// When: the publish config is pinned.
	err := PinConfigPackages(PinConfigOptions{
		RootDir:       root,
		ConfigPath:    configPath,
		RepositoryDir: repositoryDir,
		Architectures: []Architecture{ArchitectureX8664, ArchitectureAArch64},
	})

	// Then: the symlink is rejected without modifying its target.
	require.ErrorIs(t, err, errPinnedConfigNotRegular)
	assert.Equal(t, []string{"crane"}, readConfigPackages(t, victim))
}

func readConfigPackages(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var config struct {
		Contents struct {
			Packages []string `yaml:"packages"`
		} `yaml:"contents"`
	}
	require.NoError(t, yaml.Unmarshal(data, &config))
	return config.Contents.Packages
}

func writeAPKIndexArchive(t *testing.T, path, index string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	tarWriter := tar.NewWriter(gzipWriter)
	data := []byte(index)
	require.NoError(t, tarWriter.WriteHeader(&tar.Header{Name: "APKINDEX", Mode: 0o644, Size: int64(len(data))}))
	_, err := tarWriter.Write(data)
	require.NoError(t, err)
	require.NoError(t, tarWriter.Close())
	require.NoError(t, gzipWriter.Close())
	require.NoError(t, os.WriteFile(path, buffer.Bytes(), 0o644))
}
