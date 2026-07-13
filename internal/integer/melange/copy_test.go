package melange

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCopyFileRejectsSymlinkDestination(t *testing.T) {
	// Given: the public-key destination is a symlink to another writable file.
	root := t.TempDir()
	source := filepath.Join(root, "melange.rsa.pub")
	destination := filepath.Join(root, "repo", "melange.rsa.pub")
	victim := filepath.Join(root, "victim")
	writeTestFile(t, source, "public")
	writeTestFile(t, victim, "keep")
	require.NoError(t, os.MkdirAll(filepath.Dir(destination), 0o755))
	require.NoError(t, os.Symlink(victim, destination))

	// When: the package repository copies its public key.
	err := copyFile(source, destination)

	// Then: the copy is rejected and the symlink target remains unchanged.
	require.ErrorIs(t, err, errCopyDestinationNotRegular)
	data, readErr := os.ReadFile(victim)
	require.NoError(t, readErr)
	assert.Equal(t, "keep", string(data))
}

func TestCopyFileRejectsSymlinkSource(t *testing.T) {
	// Given: the staged public-key path is a symlink.
	root := t.TempDir()
	target := filepath.Join(root, "actual-key")
	source := filepath.Join(root, "melange.rsa.pub")
	destination := filepath.Join(root, "repo", "melange.rsa.pub")
	writeTestFile(t, target, "public")
	require.NoError(t, os.Symlink(target, source))

	// When: the package repository copies its public key.
	err := copyFile(source, destination)

	// Then: the non-regular source is rejected.
	require.ErrorIs(t, err, errCopySourceNotRegular)
}
