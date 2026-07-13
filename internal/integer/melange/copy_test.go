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
	err := copyFile(root, source, destination)

	// Then: the copy is rejected and the symlink target remains unchanged.
	require.ErrorIs(t, err, errCopyDestinationNotRegular)
	data, readErr := os.ReadFile(victim)
	require.NoError(t, readErr)
	assert.Equal(t, "keep", string(data))
}

func TestCopyFileRejectsSymlinkedDestinationAncestor(t *testing.T) {
	// Given: a repository ancestor redirects the public-key destination outside the managed root.
	root := t.TempDir()
	external := t.TempDir()
	source := filepath.Join(root, "melange.rsa.pub")
	destination := filepath.Join(root, "repo", "melange.rsa.pub")
	victim := filepath.Join(external, "melange.rsa.pub")
	writeTestFile(t, source, "public")
	writeTestFile(t, victim, "keep")
	require.NoError(t, os.Symlink(external, filepath.Join(root, "repo")))

	// When: the package repository copies its public key.
	err := copyFile(root, source, destination)

	// Then: the redirected write is rejected and the external file remains unchanged.
	assert.Error(t, err)
	data, readErr := os.ReadFile(victim)
	require.NoError(t, readErr)
	assert.Equal(t, "keep", string(data))
}

func TestCopyFileRetainsDestinationParentAcrossAncestorSwap(t *testing.T) {
	// Given: the repository directory is replaced with an external symlink after its root is opened.
	root := t.TempDir()
	external := t.TempDir()
	source := filepath.Join(root, "melange.rsa.pub")
	repository := filepath.Join(root, "repo")
	destination := filepath.Join(repository, "melange.rsa.pub")
	retainedRepository := filepath.Join(root, "repo-retained")
	victim := filepath.Join(external, "melange.rsa.pub")
	writeTestFile(t, source, "public")
	writeTestFile(t, destination, "old")
	writeTestFile(t, victim, "keep")
	originalHook := replaceRegularFileAfterOpenParent
	replaceRegularFileAfterOpenParent = func() {
		require.NoError(t, os.Rename(repository, retainedRepository))
		require.NoError(t, os.Symlink(external, repository))
	}
	t.Cleanup(func() { replaceRegularFileAfterOpenParent = originalHook })

	// When: the package repository copies its public key.
	err := copyFile(root, source, destination)

	// Then: the retained directory receives the key and the external target is untouched.
	require.NoError(t, err)
	retained, readErr := os.ReadFile(filepath.Join(retainedRepository, "melange.rsa.pub"))
	require.NoError(t, readErr)
	assert.Equal(t, "public", string(retained))
	externalData, readErr := os.ReadFile(victim)
	require.NoError(t, readErr)
	assert.Equal(t, "keep", string(externalData))
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
	err := copyFile(root, source, destination)

	// Then: the non-regular source is rejected.
	require.ErrorIs(t, err, errCopySourceNotRegular)
}
