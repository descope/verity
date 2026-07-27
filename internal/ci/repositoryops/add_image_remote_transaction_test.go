package repositoryops_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	ops "github.com/verity-org/verity/internal/ci/repositoryops"
)

var errInterruptedRefUpdate = errors.New("interrupted ref update")

func TestAddImageService_Run_restoresAbsentRemoteBranchAfterPullRequestFailure(t *testing.T) {
	// Given
	fixture := newRemoteGitTransactionFixture(t, false)
	before := fixture.captureState(t)

	// When
	_, err := fixture.run(t.Context(), ops.NewGitRunner(nil), malformedPullRequestRunner())

	// Then
	require.ErrorIs(t, err, ops.ErrMalformedOutput)
	fixture.requireState(t, before)
}

func TestAddImageService_Run_restoresPreexistingRemoteBranchAfterPullRequestFailure(t *testing.T) {
	// Given
	fixture := newRemoteGitTransactionFixture(t, true)
	lockPath := filepath.Join(fixture.root, ".git", "refs", "tags", "preexisting.lock")
	require.NoError(t, os.MkdirAll(filepath.Dir(lockPath), 0o700))
	require.NoError(t, os.WriteFile(lockPath, []byte("preserve exactly\n"), 0o640))
	before := fixture.captureState(t)

	// When
	_, err := fixture.run(t.Context(), ops.NewGitRunner(nil), malformedPullRequestRunner())

	// Then
	require.ErrorIs(t, err, ops.ErrMalformedOutput)
	fixture.requireState(t, before)
}

func TestAddImageService_Run_preservesConcurrentRemoteBranchMovement(t *testing.T) {
	// Given
	fixture := newRemoteGitTransactionFixture(t, false)
	before := fixture.captureState(t)
	var concurrentOID string
	github := &fakeGitHubRunner{run: func(_ context.Context, _ ops.GitHubCommand, _ int) (ops.CommandResult, error) {
		concurrentOID = fixture.moveRemoteBranchConcurrently(t)
		return ops.CommandResult{Stdout: []byte("not-a-pull-request\n")}, nil
	}}

	// When
	_, err := fixture.run(t.Context(), ops.NewGitRunner(nil), github)

	// Then
	require.Error(t, err)
	require.Contains(t, err.Error(), "remote automation branch changed concurrently")
	fixture.requireLocalState(t, before)
	require.Equal(t, concurrentOID, fixture.remoteBranchOID(t))
}

func TestAddImageService_Run_removesCreatedRefLockAndAllowsRetry(t *testing.T) {
	// Given
	fixture := newRemoteGitTransactionFixture(t, false)
	before := fixture.captureState(t)
	runner := &lockingPushRunner{delegate: ops.NewGitRunner(nil), root: fixture.root}

	// When
	_, firstErr := fixture.run(t.Context(), runner, malformedPullRequestRunner())

	// Then
	require.ErrorIs(t, firstErr, errInterruptedRefUpdate)
	fixture.requireState(t, before)
	result, retryErr := fixture.run(t.Context(), ops.NewGitRunner(nil), validPullRequestRunner())
	require.NoError(t, retryErr)
	require.Equal(t, "add-image/rclone", result.Branch)
	require.Empty(t, fixture.captureLocks(t))
}

type lockingPushRunner struct {
	delegate ops.GitRunner
	root     string
}

func (r *lockingPushRunner) Run(ctx context.Context, command ops.GitCommand) (ops.CommandResult, error) {
	if len(command.Args) > 0 && command.Args[0] == "push" {
		lock := filepath.Join(r.root, ".git", "refs", "heads", "add-image", "rclone.lock")
		if err := os.WriteFile(lock, []byte("interrupted\n"), 0o600); err != nil {
			return ops.CommandResult{}, err
		}
		return ops.CommandResult{}, errInterruptedRefUpdate
	}
	return r.delegate.Run(ctx, command)
}

func malformedPullRequestRunner() *fakeGitHubRunner {
	return &fakeGitHubRunner{run: func(_ context.Context, _ ops.GitHubCommand, _ int) (ops.CommandResult, error) {
		return ops.CommandResult{Stdout: []byte("not-a-pull-request\n")}, nil
	}}
}

func validPullRequestRunner() *fakeGitHubRunner {
	return &fakeGitHubRunner{run: func(_ context.Context, _ ops.GitHubCommand, _ int) (ops.CommandResult, error) {
		return ops.CommandResult{Stdout: []byte("https://github.com/verity-org/verity/pull/42\n")}, nil
	}}
}
