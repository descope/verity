package melange

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

var (
	errSentinelPin    = errors.New("sentinel")
	errReplacementPin = errors.New("replacement")
)

func Test_pinConfigPaths_rejects_repository_and_config_outside_root(t *testing.T) {
	// Given
	root := t.TempDir()
	outside := filepath.Join(filepath.Dir(root), "outside")

	// When
	_, _, _, repositoryErr := pinConfigPaths(PinConfigOptions{
		RootDir: root, RepositoryDir: outside, ConfigPath: filepath.Join(root, "config.yaml"),
	})
	_, _, _, configErr := pinConfigPaths(PinConfigOptions{
		RootDir: root, RepositoryDir: filepath.Join(root, "repo"), ConfigPath: outside,
	})

	// Then
	require.ErrorContains(t, repositoryErr, "locate local package repository")
	require.ErrorContains(t, configErr, "locate apko config")
}

func Test_loadPinnedPackages_reports_architecture_index_and_version_boundaries(t *testing.T) {
	tests := []struct {
		name      string
		arch      Architecture
		setup     func(t *testing.T, indexPath string)
		wantError error
		contains  string
	}{
		{name: "unsupported architecture", arch: Architecture("unsupported"), wantError: errUnsupportedArchitecture},
		{name: "missing index", arch: ArchitectureX8664, contains: "read local package index"},
		{
			name: "index is a directory",
			arch: ArchitectureX8664,
			setup: func(t *testing.T, indexPath string) {
				require.NoError(t, os.MkdirAll(indexPath, 0o755))
			},
			wantError: errPinnedIndexNotRegular,
		},
		{
			name: "invalid index archive",
			arch: ArchitectureX8664,
			setup: func(t *testing.T, indexPath string) {
				writeTestFile(t, indexPath, "not an archive")
			},
			contains: "parse local package index",
		},
		{
			name: "package version is empty",
			arch: ArchitectureX8664,
			setup: func(t *testing.T, indexPath string) {
				writeAPKIndexArchive(t, indexPath, "P:sentinel\nV:\n\n")
			},
			wantError: errPinnedPackageVersionUndefined,
		},
		{
			name: "duplicate versions conflict within one architecture",
			arch: ArchitectureX8664,
			setup: func(t *testing.T, indexPath string) {
				writeAPKIndexArchive(t, indexPath, "P:sentinel\nV:1.0-r0\n\nP:sentinel\nV:2.0-r0\n\n")
			},
			wantError: errPinnedPackageVersionConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given
			root := t.TempDir()
			repository := filepath.Join(root, "repo")
			indexPath := filepath.Join(repository, string(tt.arch), "APKINDEX.tar.gz")
			if tt.setup != nil {
				tt.setup(t, indexPath)
			}

			// When
			_, err := loadPinnedPackages(root, "repo", []Architecture{tt.arch})

			// Then
			if tt.wantError != nil {
				require.ErrorIs(t, err, tt.wantError)
			} else {
				require.ErrorContains(t, err, tt.contains)
			}
		})
	}
}

func Test_PinConfigPackages_reports_config_file_and_yaml_shape_boundaries(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(t *testing.T, configPath string)
		wantError error
		contains  string
	}{
		{name: "missing config", contains: "inspect apko config"},
		{
			name: "config is a directory",
			setup: func(t *testing.T, configPath string) {
				require.NoError(t, os.MkdirAll(configPath, 0o755))
			},
			wantError: errPinnedConfigNotRegular,
		},
		{
			name:     "invalid yaml",
			setup:    func(t *testing.T, configPath string) { writeTestFile(t, configPath, "contents: [") },
			contains: "parse apko config",
		},
		{
			name:      "contents missing",
			setup:     func(t *testing.T, configPath string) { writeTestFile(t, configPath, "other: true\n") },
			wantError: errPinnedConfigPackagesMissing,
		},
		{
			name: "packages is not a sequence",
			setup: func(t *testing.T, configPath string) {
				writeTestFile(t, configPath, "contents:\n  packages: sentinel\n")
			},
			wantError: errPinnedConfigPackagesMissing,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given
			root := t.TempDir()
			configPath := filepath.Join(root, "config.apko.yaml")
			repository := filepath.Join(root, "repo")
			writeAPKIndexArchive(t, filepath.Join(repository, string(ArchitectureX8664), "APKINDEX.tar.gz"), "P:sentinel\nV:1.0-r0\n\n")
			if tt.setup != nil {
				tt.setup(t, configPath)
			}

			// When
			err := PinConfigPackages(PinConfigOptions{
				RootDir: root, ConfigPath: configPath, RepositoryDir: repository,
				Architectures: []Architecture{ArchitectureX8664},
			})

			// Then
			if tt.wantError != nil {
				require.ErrorIs(t, err, tt.wantError)
			} else {
				require.ErrorContains(t, err, tt.contains)
			}
		})
	}
}

func Test_pinnedRegularFileError_and_yaml_node_helpers_preserve_boundary_meaning(t *testing.T) {
	// Given
	// When / Then
	for _, source := range []error{errNotRegularFile, errPathContainsSymlink, errNotRealDirectory} {
		assert.ErrorIs(t, pinnedRegularFileError(source, errReplacementPin), errReplacementPin)
	}
	assert.ErrorIs(t, pinnedRegularFileError(errSentinelPin, errReplacementPin), errSentinelPin)

	nonMapping := &yaml.Node{Kind: yaml.SequenceNode}
	assert.Nil(t, yamlMappingValue(nonMapping, "contents"))
	mapping := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{
		{Kind: yaml.ScalarNode, Value: "other"}, {Kind: yaml.ScalarNode, Value: "value"},
	}}
	assert.Nil(t, yamlMappingValue(mapping, "contents"))

	_, err := configPackagesNode(&yaml.Node{Kind: yaml.SequenceNode})
	require.ErrorIs(t, err, errPinnedConfigPackagesMissing)
}
