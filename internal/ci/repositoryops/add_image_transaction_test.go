package repositoryops_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	ops "github.com/verity-org/verity/internal/ci/repositoryops"
)

var errCommitRejected = errors.New("commit rejected")

func TestAddImageService_Run_restoresEveryWorktreeFileWhenLaterMutationFails(t *testing.T) {
	// Given
	fixture := newAddImageRollbackFixture(t)
	git := fixture.gitRunner(func(command string) error {
		if strings.HasPrefix(command, "checkout -b ") {
			fixture.mutateWorktree(t)
		}
		if strings.HasPrefix(command, "commit -m ") {
			return errCommitRejected
		}
		return nil
	})

	// When
	_, err := (ops.AddImageService{Git: git, GitHub: &fakeGitHubRunner{}}).Run(t.Context(), &fixture.request)

	// Then
	require.ErrorIs(t, err, errCommitRejected)
	fixture.requireOriginalWorktree(t)
}

func TestAddImageService_Run_restoresEveryWorktreeFileWhenCommandIsCanceled(t *testing.T) {
	// Given
	fixture := newAddImageRollbackFixture(t)
	git := fixture.gitRunner(func(command string) error {
		if strings.HasPrefix(command, "checkout -b ") {
			fixture.mutateWorktree(t)
		}
		if strings.HasPrefix(command, "push -u origin ") {
			return context.Canceled
		}
		return nil
	})

	// When
	_, err := (ops.AddImageService{Git: git, GitHub: &fakeGitHubRunner{}}).Run(t.Context(), &fixture.request)

	// Then
	require.ErrorIs(t, err, context.Canceled)
	fixture.requireOriginalWorktree(t)
}

func TestAddImageService_Run_restoresEveryWorktreeFileWhenGitHubOutputIsForeign(t *testing.T) {
	// Given
	fixture := newAddImageRollbackFixture(t)
	git := fixture.gitRunner(func(command string) error {
		if strings.HasPrefix(command, "checkout -b ") {
			fixture.mutateWorktree(t)
		}
		return nil
	})
	github := &fakeGitHubRunner{run: func(_ context.Context, _ ops.GitHubCommand, _ int) (ops.CommandResult, error) {
		return ops.CommandResult{Stdout: []byte("https://github.com/attacker/other/pull/42\n")}, nil
	}}

	// When
	_, err := (ops.AddImageService{Git: git, GitHub: github}).Run(t.Context(), &fixture.request)

	// Then
	require.ErrorIs(t, err, ops.ErrMalformedOutput)
	fixture.requireOriginalWorktree(t)
}

type addImageRollbackFixture struct {
	root              string
	configPath        string
	dirtyPath         string
	untrackedPath     string
	createdPath       string
	configOriginal    []byte
	dirtyOriginal     []byte
	untrackedOriginal []byte
	request           ops.AddImageRequest
	gitMetadata       *fakeGitMetadata
}

func newAddImageRollbackFixture(t *testing.T) *addImageRollbackFixture {
	t.Helper()
	root := t.TempDir()
	fixture := &addImageRollbackFixture{
		root:              root,
		configPath:        filepath.Join(root, "copa-config.yaml"),
		dirtyPath:         filepath.Join(root, "README.md"),
		untrackedPath:     filepath.Join(root, "local-notes.txt"),
		createdPath:       filepath.Join(root, "generated-by-hook.txt"),
		configOriginal:    []byte("# preserve formatting\nimages: []\n"),
		dirtyOriginal:     []byte("dirty pre-state\n"),
		untrackedOriginal: []byte("untracked pre-state\n"),
	}
	require.NoError(t, os.WriteFile(fixture.configPath, fixture.configOriginal, 0o640))
	require.NoError(t, os.WriteFile(fixture.dirtyPath, fixture.dirtyOriginal, 0o600))
	require.NoError(t, os.WriteFile(fixture.untrackedPath, fixture.untrackedOriginal, 0o644))
	fixture.gitMetadata = newFakeGitMetadata(t, root)
	issue, err := ops.ParseImageIssue("### Image name\nrclone\n\n### Image repository\nrclone/rclone\n\n### Image tag\nv1.70.3\n")
	require.NoError(t, err)
	fixture.request, err = ops.NewAddImageRequest(&ops.AddImageRequestInput{
		RepositoryRoot: root, GitHubRepository: expectedGitHubRepository,
		ConfigPath: fixture.configPath, Issue: issue, IssueNumber: "123", BaseBranch: "main",
	})
	require.NoError(t, err)
	return fixture
}

func (f *addImageRollbackFixture) gitRunner(afterCommand func(string) error) *fakeGitRunner {
	automationCreated := false
	return &fakeGitRunner{run: func(_ context.Context, command ops.GitCommand, _ int) (ops.CommandResult, error) {
		joined := strings.Join(command.Args, " ")
		if joined == "rev-parse --verify --quiet refs/heads/add-image/rclone" && automationCreated {
			return ops.CommandResult{Stdout: []byte(fakeGitHeadOID + "\n")}, nil
		}
		if result, handled := f.gitMetadata.response(command); handled {
			return result, nil
		}
		switch {
		case joined == "status --porcelain=v1 -z --untracked-files=all":
			return ops.CommandResult{}, nil
		case strings.HasPrefix(joined, "show-ref --verify --quiet refs/heads/add-image/rclone"):
			return ops.CommandResult{ExitCode: 1}, nil
		case strings.HasPrefix(joined, "config user."), strings.HasPrefix(joined, "checkout -b "), strings.HasPrefix(joined, "add -- "), strings.HasPrefix(joined, "commit -m "), strings.HasPrefix(joined, "push -u origin "):
			if strings.HasPrefix(joined, "checkout -b ") {
				automationCreated = true
			}
			if err := afterCommand(joined); err != nil {
				return ops.CommandResult{}, err
			}
			return ops.CommandResult{}, nil
		default:
			return ops.CommandResult{}, fmt.Errorf("%w: git %s", errUnexpectedFakeCommand, joined)
		}
	}}
}

func (f *addImageRollbackFixture) mutateWorktree(t *testing.T) {
	t.Helper()
	require.NoError(t, os.WriteFile(f.dirtyPath, []byte("overwritten\n"), 0o600))
	require.NoError(t, os.WriteFile(f.untrackedPath, []byte("overwritten\n"), 0o644))
	require.NoError(t, os.WriteFile(f.createdPath, []byte("created\n"), 0o600))
	require.NoError(t, os.Chmod(f.dirtyPath, 0o777))
	require.NoError(t, os.Chmod(f.untrackedPath, 0o400))
}

func (f *addImageRollbackFixture) requireOriginalWorktree(t *testing.T) {
	t.Helper()
	requireFileBytes(t, f.configPath, f.configOriginal)
	requireFileBytes(t, f.dirtyPath, f.dirtyOriginal)
	requireFileBytes(t, f.untrackedPath, f.untrackedOriginal)
	requireFileMode(t, f.configPath, 0o640)
	requireFileMode(t, f.dirtyPath, 0o600)
	requireFileMode(t, f.untrackedPath, 0o644)
	_, err := os.Lstat(f.createdPath)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func requireFileBytes(t *testing.T, path string, expected []byte) {
	t.Helper()
	actual, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, expected, actual)
}

func requireFileMode(t *testing.T, path string, expected os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, expected, info.Mode().Perm())
}
