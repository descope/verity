package repositoryops

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGitFileSnapshot_restore_removesFileCreatedAfterMissingSnapshot(t *testing.T) {
	// Given
	path := filepath.Join(t.TempDir(), "transient.lock")
	snapshot, err := captureGitFile(gitFileSnapshotRequest{path: path, label: "transient lock", required: false})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, []byte("created\n"), 0o600))

	// When
	err = snapshot.restore()

	// Then
	require.NoError(t, err)
	_, statErr := os.Stat(path)
	assert.True(t, errors.Is(statErr, os.ErrNotExist))
}

func TestCaptureGitFile_rejectsRelativeMetadataPath(t *testing.T) {
	// When
	_, err := captureGitFile(gitFileSnapshotRequest{path: "relative/index", label: "index", required: true})

	// Then
	require.ErrorIs(t, err, ErrUnsupportedGitState)
}

func TestGitDirectoryRoots_rejectsMetadataFile(t *testing.T) {
	// Given
	path := filepath.Join(t.TempDir(), "not-a-directory")
	require.NoError(t, os.WriteFile(path, []byte("sentinel\n"), 0o600))

	// When
	_, err := gitDirectoryRoots(path)

	// Then
	require.ErrorIs(t, err, ErrUnsupportedGitState)
}
