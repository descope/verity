package repositoryops_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ops "github.com/verity-org/verity/internal/ci/repositoryops"
)

var errForcedPushFailure = errors.New("forced push failure")

func TestAddImageService_Run_restoresRealGitStateAfterPostCheckoutFailure(t *testing.T) {
	// Given
	fixture := newRealGitTransactionFixture(t, false)
	before := fixture.captureState(t)
	runner := &transactionGitRunner{delegate: ops.NewGitRunner(nil), pushError: errForcedPushFailure}

	// When
	_, err := (ops.AddImageService{Git: runner, GitHub: &fakeGitHubRunner{}}).Run(t.Context(), &fixture.request)

	// Then
	require.ErrorIs(t, err, errForcedPushFailure)
	fixture.requireState(t, before)
}

func TestAddImageService_Run_restoresRealGitStateAfterCancellation(t *testing.T) {
	// Given
	fixture := newRealGitTransactionFixture(t, true)
	before := fixture.captureState(t)
	ctx, cancel := context.WithCancel(t.Context())
	runner := &transactionGitRunner{delegate: ops.NewGitRunner(nil), cancelPush: cancel}

	// When
	_, err := (ops.AddImageService{Git: runner, GitHub: &fakeGitHubRunner{}}).Run(ctx, &fixture.request)

	// Then
	require.ErrorIs(t, err, context.Canceled)
	fixture.requireState(t, before)
}

type transactionGitRunner struct {
	delegate    ops.GitRunner
	pushError   error
	cancelPush  context.CancelFunc
	statusCalls int
}

func (r *transactionGitRunner) Run(ctx context.Context, command ops.GitCommand) (ops.CommandResult, error) {
	joined := strings.Join(command.Args, " ")
	if joined == "status --porcelain=v1 -z --untracked-files=all" && r.statusCalls == 0 {
		r.statusCalls++
		return ops.CommandResult{}, nil
	}
	if strings.HasPrefix(joined, "push -u origin ") {
		if r.cancelPush != nil {
			r.cancelPush()
			return r.delegate.Run(ctx, command)
		}
		return ops.CommandResult{}, r.pushError
	}
	return r.delegate.Run(ctx, command)
}

type realGitTransactionFixture struct {
	root      string
	files     []string
	branchRef string
	request   ops.AddImageRequest
}

type repositoryState struct {
	head             string
	symbolicRef      string
	hasSymbolicRef   bool
	status           []byte
	index            fileDigest
	config           fileDigest
	automationRef    string
	hasAutomationRef bool
	files            map[string]fileState
}

type fileDigest struct {
	exists bool
	value  [sha256.Size]byte
}

type fileState struct {
	data []byte
	mode os.FileMode
}

func newRealGitTransactionFixture(t *testing.T, detached bool) *realGitTransactionFixture {
	t.Helper()
	root := t.TempDir()
	runGit(t, root, "init", "-b", "main")
	paths := []string{"copa-config.yaml", "staged.txt", "unstaged.txt", "untracked.txt"}
	require.NoError(t, os.WriteFile(filepath.Join(root, paths[0]), []byte("# exact catalog\nimages: []\n"), 0o640))
	require.NoError(t, os.WriteFile(filepath.Join(root, paths[1]), []byte("staged baseline\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, paths[2]), []byte("unstaged baseline\n"), 0o644))
	runGit(t, root, "add", "--", paths[0], paths[1], paths[2])
	runGit(t, root, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.com", "commit", "-m", "baseline")
	remote := filepath.Join(t.TempDir(), "origin.git")
	runGit(t, filepath.Dir(remote), "init", "--bare", remote)
	runGit(t, root, "remote", "add", "origin", remote)
	runGit(t, root, "push", "-u", "origin", "main")
	if detached {
		runGit(t, root, "checkout", "--detach", "HEAD")
	}
	require.NoError(t, os.WriteFile(filepath.Join(root, paths[1]), []byte("staged dirty\n"), 0o644))
	require.NoError(t, os.Chmod(filepath.Join(root, paths[1]), 0o640))
	runGit(t, root, "add", "--", paths[1])
	require.NoError(t, os.WriteFile(filepath.Join(root, paths[2]), []byte("unstaged dirty\n"), 0o644))
	require.NoError(t, os.Chmod(filepath.Join(root, paths[2]), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, paths[3]), []byte("untracked dirty\n"), 0o660))
	issue, err := ops.ParseImageIssue("### Image name\nrclone\n\n### Image repository\nrclone/rclone\n\n### Image tag\nv1.70.3\n")
	require.NoError(t, err)
	request, err := ops.NewAddImageRequest(&ops.AddImageRequestInput{
		RepositoryRoot: root, GitHubRepository: expectedGitHubRepository,
		ConfigPath: paths[0], Issue: issue, IssueNumber: "123", BaseBranch: "main",
	})
	require.NoError(t, err)
	return &realGitTransactionFixture{
		root: root, files: paths, branchRef: "refs/heads/add-image/rclone", request: request,
	}
}

func (f *realGitTransactionFixture) captureState(t *testing.T) *repositoryState {
	t.Helper()
	symbolicRef, hasSymbolicRef := optionalGitOutput(t, f.root, "symbolic-ref", "-q", "HEAD")
	automationRef, hasAutomationRef := optionalGitOutput(t, f.root, "rev-parse", "--verify", "--quiet", f.branchRef)
	index := digestGitPath(t, f.root, "index")
	config := digestGitPath(t, f.root, "config")
	status := runGit(t, f.root, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	state := repositoryState{
		head:        strings.TrimSpace(string(runGit(t, f.root, "rev-parse", "HEAD"))),
		symbolicRef: strings.TrimSpace(symbolicRef), hasSymbolicRef: hasSymbolicRef,
		status: status, index: index, config: config,
		automationRef: strings.TrimSpace(automationRef), hasAutomationRef: hasAutomationRef,
		files: make(map[string]fileState, len(f.files)),
	}
	for _, relative := range f.files {
		path := filepath.Join(f.root, relative)
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		info, err := os.Stat(path)
		require.NoError(t, err)
		state.files[relative] = fileState{data: data, mode: info.Mode().Perm()}
	}
	return &state
}

func (f *realGitTransactionFixture) requireState(t *testing.T, expected *repositoryState) {
	t.Helper()
	actual := f.captureState(t)
	assert.Equal(t, expected.head, actual.head, "rev-parse HEAD")
	assert.Equal(t, expected.hasSymbolicRef, actual.hasSymbolicRef, "symbolic-ref presence")
	assert.Equal(t, expected.symbolicRef, actual.symbolicRef, "symbolic-ref HEAD")
	assert.Equal(t, expected.status, actual.status, "git status --porcelain=v1 -z")
	assert.Equal(t, expected.index, actual.index, "index digest")
	assert.Equal(t, expected.config, actual.config, "git config digest")
	assert.Equal(t, expected.hasAutomationRef, actual.hasAutomationRef, "automation ref presence")
	assert.Equal(t, expected.automationRef, actual.automationRef, "automation ref OID")
	assert.Equal(t, expected.files, actual.files, "worktree bytes and modes")
}

func digestGitPath(t *testing.T, root, name string) fileDigest {
	t.Helper()
	path := strings.TrimSpace(string(runGit(t, root, "rev-parse", "--path-format=absolute", "--git-path", name)))
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return fileDigest{}
	}
	require.NoError(t, err)
	return fileDigest{exists: true, value: sha256.Sum256(data)}
}

func runGit(t *testing.T, root string, args ...string) []byte {
	t.Helper()
	command := exec.CommandContext(t.Context(), "git", args...)
	command.Dir = root
	output, err := command.CombinedOutput()
	require.NoError(t, err, string(output))
	return output
}

func optionalGitOutput(t *testing.T, root string, args ...string) (string, bool) {
	t.Helper()
	command := exec.CommandContext(t.Context(), "git", args...)
	command.Dir = root
	output, err := command.CombinedOutput()
	if err == nil {
		return string(output), true
	}
	var exitError *exec.ExitError
	require.ErrorAs(t, err, &exitError)
	require.Equal(t, 1, exitError.ExitCode(), string(output))
	return "", false
}
