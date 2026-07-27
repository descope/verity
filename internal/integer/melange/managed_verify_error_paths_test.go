package melange

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_validateDirectoryChain_rejects_missing_file_and_symlink_components(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T) string
		want  error
	}{
		{
			name: "missing",
			setup: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "missing")
			},
		},
		{
			name: "file",
			setup: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "file")
				writeTestFile(t, path, "sentinel")
				return path
			},
			want: errInvalidRoot,
		},
		{
			name: "symlink",
			setup: func(t *testing.T) string {
				root := t.TempDir()
				target := filepath.Join(root, "target")
				require.NoError(t, os.Mkdir(target, 0o755))
				link := filepath.Join(root, "link")
				require.NoError(t, os.Symlink(target, link))
				return link
			},
			want: errInvalidRoot,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given
			path := tt.setup(t)

			// When
			err := validateDirectoryChain(path)

			// Then
			if tt.want != nil {
				require.ErrorIs(t, err, tt.want)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func Test_relativeToRoot_and_ensureManagedDirectory_reject_escape_and_non_directory_targets(t *testing.T) {
	// Given
	root := t.TempDir()
	outside := filepath.Join(filepath.Dir(root), "outside")
	fileTarget := filepath.Join(root, "packages")
	writeTestFile(t, fileTarget, "sentinel")

	// When
	_, relativeErr := relativeToRoot(root, outside)
	_, _, escapeErr := ensureManagedDirectory(root, outside)
	_, _, fileErr := ensureManagedDirectory(root, filepath.Join(fileTarget, "repo"))

	// Then
	require.ErrorIs(t, relativeErr, errUnsafeRelativePath)
	require.ErrorIs(t, escapeErr, errUnsafeRelativePath)
	require.ErrorIs(t, fileErr, errInvalidRoot)
}

func Test_validateRootDirectories_handles_missing_file_and_symlink_boundaries(t *testing.T) {
	// Given
	rootPath := t.TempDir()
	root, err := os.OpenRoot(rootPath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, root.Close()) })
	writeTestFile(t, filepath.Join(rootPath, "file"), "sentinel")
	require.NoError(t, os.Symlink("missing", filepath.Join(rootPath, "link")))

	// When / Then
	require.NoError(t, validateRootDirectories(root, "missing/child", true))
	require.Error(t, validateRootDirectories(root, "missing/child", false))
	require.ErrorIs(t, validateRootDirectories(root, "file/child", false), errInvalidRoot)
	require.ErrorIs(t, validateRootDirectories(root, "link/child", false), errInvalidRoot)
}

func Test_regularFile_and_secureOptionalDir_report_unsafe_missing_and_nonregular_paths(t *testing.T) {
	// Given
	root := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, "directory"), 0o755))
	writeTestFile(t, filepath.Join(root, "plain"), "sentinel")

	// When / Then
	_, err := regularFile(root, "../escape")
	require.ErrorIs(t, err, errUnsafeRelativePath)
	_, err = regularFile(root, "missing")
	require.Error(t, err)
	_, err = regularFile(root, "directory")
	require.ErrorIs(t, err, errNotRegularFile)

	missing, err := secureOptionalDir(root, "missing")
	require.NoError(t, err)
	assert.Empty(t, missing)
	_, err = secureOptionalDir(root, "../escape")
	require.ErrorIs(t, err, errUnsafeRelativePath)
	_, err = secureOptionalDir(root, "plain")
	require.ErrorIs(t, err, errNotRealDirectory)
}

func Test_securePath_returns_root_for_empty_path_and_rejects_symlinks(t *testing.T) {
	// Given
	root := t.TempDir()
	require.NoError(t, os.Symlink("missing", filepath.Join(root, "link")))

	// When
	empty, emptyErr := securePath(root, "")
	missing, missingErr := securePath(root, "missing/child")
	_, linkErr := securePath(root, "link/child")

	// Then
	require.NoError(t, emptyErr)
	assert.Equal(t, root, empty)
	require.NoError(t, missingErr)
	assert.Equal(t, filepath.Join(root, "missing", "child"), missing)
	require.ErrorIs(t, linkErr, errPathContainsSymlink)
}

func Test_treeFiles_handles_empty_missing_and_invalid_entries(t *testing.T) {
	// Given / When / Then
	files, err := treeFiles("", "prefix")
	require.NoError(t, err)
	assert.Nil(t, files)

	missing := filepath.Join(t.TempDir(), "missing")
	files, err = treeFiles(missing, "prefix")
	require.NoError(t, err)
	assert.Nil(t, files)

	root := t.TempDir()
	require.NoError(t, os.Symlink("missing", filepath.Join(root, "bad-link")))
	_, err = treeFiles(root, "prefix")
	require.ErrorIs(t, err, errInvalidTreeEntry)
}

func Test_readRegularFile_rejects_invalid_root_and_directory_targets(t *testing.T) {
	// Given
	rootFile := filepath.Join(t.TempDir(), "root-file")
	writeTestFile(t, rootFile, "sentinel")
	root := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, "directory"), 0o755))

	// When
	_, rootErr := readRegularFile(rootFile, "child")
	_, directoryErr := readRegularFile(root, "directory")

	// Then
	require.ErrorIs(t, rootErr, errInvalidRoot)
	require.ErrorIs(t, directoryErr, errNotRegularFile)
}
