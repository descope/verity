package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadImageEntryRejectsSymlinkSwapAfterInventory(t *testing.T) {
	dir := t.TempDir()
	definitionPath := filepath.Join(dir, "node.yaml")
	require.NoError(t, os.WriteFile(definitionPath, []byte("name: node\n"), 0o644))

	root, err := os.OpenRoot(dir)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, root.Close()) })

	entries, err := imageFileEntries(root, dir)
	require.NoError(t, err)
	require.Len(t, entries, 1)

	outside := filepath.Join(t.TempDir(), "outside.yaml")
	require.NoError(t, os.WriteFile(outside, []byte("name: escaped\n"), 0o644))
	require.NoError(t, os.Rename(definitionPath, definitionPath+".original"))
	require.NoError(t, os.Symlink(outside, definitionPath))

	_, err = loadImageEntry(root, entries[0])
	require.ErrorIs(t, err, ErrInvalidImageFile)
}
