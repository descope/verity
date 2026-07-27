package repositoryops

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddImageTransaction_restore_reinstatesRepositoryAfterCanceledFailure(t *testing.T) {
	// Given
	fixture := newRollbackGitFixture(t)
	before := captureRollbackRepositoryState(t, fixture)
	beforeIndex := readRollbackFile(t, filepath.Join(fixture.root, ".git", "index"))
	git := NewGitRunner(nil)
	snapshot, err := captureAddImageTransaction(t.Context(), git, &AddImageRequest{
		repoRoot: fixture.root,
		branch:   fixture.branch,
	})
	require.NoError(t, err)
	runRepositoryGit(t, fixture.root, "checkout", "-b", fixture.branch)
	runRepositoryGit(t, fixture.root, "config", "transaction.changed", "true")
	require.NoError(t, os.Remove(fixture.preservedLock))
	require.NoError(t, os.RemoveAll(filepath.Join(fixture.root, "nested")))
	require.NoError(t, os.WriteFile(filepath.Join(fixture.root, "nested"), []byte("directory became file\n"), 0o644))
	require.NoError(t, os.Remove(filepath.Join(fixture.root, "tracked-link")))
	require.NoError(t, os.Mkdir(filepath.Join(fixture.root, "tracked-link"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(fixture.root, "tracked-link", "child"), []byte("created\n"), 0o644))
	require.NoError(t, os.Remove(filepath.Join(fixture.root, "tracked.txt")))
	require.NoError(t, os.Mkdir(filepath.Join(fixture.root, "tracked.txt"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(fixture.root, "tracked.txt", "child"), []byte("created\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(fixture.root, "created", "deep"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(fixture.root, "created", "deep", "new.txt"), []byte("new\n"), 0o644))
	runRepositoryGit(t, fixture.root, "add", "-A")
	require.NoError(t, snapshot.git.remote.prepare(t.Context(), git, "refs/heads/"+fixture.branch))
	runRepositoryGit(t, fixture.root, "push", "-u", "origin", fixture.branch)
	require.NoError(t, os.WriteFile(filepath.Join(fixture.root, ".git", "created.lock"), []byte("created-lock\n"), 0o600))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	// When
	err = snapshot.restore(ctx, git)

	// Then
	require.NoError(t, err)
	assert.Equal(t, beforeIndex, readRollbackFile(t, filepath.Join(fixture.root, ".git", "index")))
	after := captureRollbackRepositoryState(t, fixture)
	assert.Equal(t, before, after)
}

func TestGitRepositorySnapshot_restore_preservesConcurrentLocalBranch(t *testing.T) {
	// Given
	fixture := newRollbackGitFixture(t)
	git := NewGitRunner(nil)
	snapshot, err := captureGitRepository(t.Context(), gitStateReader{git: git, root: fixture.root}, "refs/heads/"+fixture.branch)
	require.NoError(t, err)
	runRepositoryGit(t, fixture.root, "checkout", "-b", fixture.branch)
	require.NoError(t, os.WriteFile(filepath.Join(fixture.root, "concurrent.txt"), []byte("concurrent\n"), 0o600))
	runRepositoryGit(t, fixture.root, "add", "--", "concurrent.txt")
	runRepositoryGit(t, fixture.root, "-c", "user.name=Concurrent", "-c", "user.email=concurrent@example.com", "commit", "-m", "concurrent change")
	concurrentOID := runRepositoryGit(t, fixture.root, "rev-parse", "refs/heads/"+fixture.branch)

	// When
	err = snapshot.restore(t.Context(), git)

	// Then
	require.ErrorIs(t, err, ErrGitRollback)
	require.ErrorIs(t, err, ErrConcurrentLocalChange)
	assert.Equal(t, concurrentOID, runRepositoryGit(t, fixture.root, "rev-parse", "refs/heads/"+fixture.branch))
}

func TestCaptureWorktree_rejectsUnsupportedFIFO(t *testing.T) {
	// Given
	root := t.TempDir()
	require.NoError(t, syscall.Mkfifo(filepath.Join(root, "sentinel.fifo"), 0o600))

	// When
	_, err := captureWorktree(root)

	// Then
	require.ErrorIs(t, err, ErrWorktreeSnapshot)
	require.ErrorIs(t, err, ErrUnsupportedWorktreeEntry)
}

func TestGitFileSnapshot_restore_rejectsDirectoryCollision(t *testing.T) {
	// Given
	path := filepath.Join(t.TempDir(), "index")
	require.NoError(t, os.WriteFile(path, []byte("original\n"), 0o600))
	snapshot, err := captureGitFile(gitFileSnapshotRequest{path: path, label: "index", required: true})
	require.NoError(t, err)
	require.NoError(t, os.Remove(path))
	require.NoError(t, os.Mkdir(path, 0o700))

	// When
	err = snapshot.restore()

	// Then
	require.ErrorIs(t, err, ErrUnsupportedGitState)
	info, statErr := os.Stat(path)
	require.NoError(t, statErr)
	assert.True(t, info.IsDir())
}

func TestAppendRefRestore_rejectsMissingTransactionRef(t *testing.T) {
	// Given
	state := &refRestore{
		ref: "refs/heads/add-image/sentinel", oid: "2222222222222222222222222222222222222222", existed: true, currentExists: false,
		transactionOID: "1111111111111111111111111111111111111111",
	}

	// When
	err := appendRefRestore(&strings.Builder{}, state)

	// Then
	require.True(t, errors.Is(err, ErrConcurrentLocalChange))
}
