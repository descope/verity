package melange

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_copyFile_reports_source_escape_missing_source_and_nonregular_destination(t *testing.T) {
	// Given
	root := t.TempDir()
	source := filepath.Join(root, "source")
	writeTestFile(t, source, "sentinel")
	destinationDirectory := filepath.Join(root, "destination")
	require.NoError(t, os.Mkdir(destinationDirectory, 0o755))

	// When / Then
	require.ErrorIs(t, copyFile(root, filepath.Join(filepath.Dir(root), "outside"), filepath.Join(root, "copy")), errCopySourceNotRegular)
	require.ErrorContains(t, copyFile(root, filepath.Join(root, "missing"), filepath.Join(root, "copy")), "read")
	require.ErrorIs(t, copyFile(root, source, destinationDirectory), errCopyDestinationNotRegular)
}

func Test_replaceRegularFile_reports_invalid_root_and_existing_directory(t *testing.T) {
	// Given
	rootFile := filepath.Join(t.TempDir(), "root-file")
	writeTestFile(t, rootFile, "sentinel")
	root := t.TempDir()
	destination := filepath.Join(root, "destination")
	require.NoError(t, os.Mkdir(destination, 0o755))

	// When
	rootErr := replaceRegularFile(rootFile, filepath.Join(rootFile, "child"), []byte("data"), 0o644, errCopyDestinationNotRegular)
	destinationErr := replaceRegularFile(root, destination, []byte("data"), 0o644, errCopyDestinationNotRegular)

	// Then
	require.ErrorIs(t, rootErr, errInvalidRoot)
	require.ErrorIs(t, destinationErr, errCopyDestinationNotRegular)
}

func Test_removeTemporaryRegularFile_ignores_missing_and_reports_nonempty_directory(t *testing.T) {
	// Given
	rootPath := t.TempDir()
	root, err := os.OpenRoot(rootPath)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, root.Close()) })

	// When / Then
	require.NoError(t, removeTemporaryRegularFile(root, "missing", "destination"))
	writeTestFile(t, filepath.Join(rootPath, "temporary", "child"), "sentinel")
	require.ErrorContains(t, removeTemporaryRegularFile(root, "temporary", "destination"), "remove temporary")
}
