package repositoryops_test

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	ops "github.com/verity-org/verity/internal/ci/repositoryops"
)

type remoteGitTransactionFixture struct {
	root      string
	remote    string
	branchRef string
	request   ops.AddImageRequest
}

type completeRepositoryState struct {
	localRefs  string
	remoteRefs string
	head       string
	symbolic   string
	status     []byte
	index      fileDigest
	config     fileDigest
	locks      map[string]fileState
	files      map[string]fileState
}

func newRemoteGitTransactionFixture(t *testing.T, preexistingBranch bool) *remoteGitTransactionFixture {
	t.Helper()
	base := t.TempDir()
	root := filepath.Join(base, "work")
	remote := filepath.Join(base, "origin.git")
	require.NoError(t, os.Mkdir(root, 0o700))
	runGit(t, base, "init", "--bare", remote)
	runGit(t, root, "init", "-b", "main")
	require.NoError(t, os.WriteFile(filepath.Join(root, "copa-config.yaml"), []byte("images: []\n"), 0o640))
	require.NoError(t, os.WriteFile(filepath.Join(root, "tracked.txt"), []byte("baseline\n"), 0o600))
	runGit(t, root, "add", "--", "copa-config.yaml", "tracked.txt")
	runGit(t, root, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.com", "commit", "-m", "baseline")
	runGit(t, root, "remote", "add", "origin", remote)
	runGit(t, root, "push", "-u", "origin", "main")
	if preexistingBranch {
		runGit(t, root, "push", "origin", "HEAD:refs/heads/add-image/rclone")
	}
	issue, err := ops.ParseImageIssue("### Image name\nrclone\n\n### Image repository\nrclone/rclone\n\n### Image tag\nv1.70.3\n")
	require.NoError(t, err)
	request, err := ops.NewAddImageRequest(&ops.AddImageRequestInput{
		RepositoryRoot: root, GitHubRepository: expectedGitHubRepository,
		ConfigPath: "copa-config.yaml", Issue: issue, IssueNumber: "123", BaseBranch: "main",
	})
	require.NoError(t, err)
	return &remoteGitTransactionFixture{
		root: root, remote: remote, branchRef: "refs/heads/add-image/rclone", request: request,
	}
}

func (f *remoteGitTransactionFixture) run(
	ctx context.Context,
	git ops.GitRunner,
	github ops.GitHubRunner,
) (ops.AddImageResult, error) {
	return (ops.AddImageService{Git: git, GitHub: github}).Run(ctx, &f.request)
}

func (f *remoteGitTransactionFixture) captureState(t *testing.T) *completeRepositoryState {
	t.Helper()
	index := digestGitPath(t, f.root, "index")
	config := digestGitPath(t, f.root, "config")
	return &completeRepositoryState{
		localRefs:  string(runGit(t, f.root, "for-each-ref", "--format=%(refname) %(objectname)")),
		remoteRefs: string(runGit(t, f.remote, "for-each-ref", "--format=%(refname) %(objectname)")),
		head:       strings.TrimSpace(string(runGit(t, f.root, "rev-parse", "HEAD"))),
		symbolic:   optionalSymbolicRef(t, f.root),
		status:     runGit(t, f.root, "status", "--porcelain=v1", "-z", "--untracked-files=all"),
		index:      index,
		config:     config,
		locks:      f.captureLocks(t),
		files:      captureFixtureFiles(t, f.root),
	}
}

func (f *remoteGitTransactionFixture) requireState(t *testing.T, expected *completeRepositoryState) {
	t.Helper()
	actual := f.captureState(t)
	require.Equal(t, expected, actual)
}

func (f *remoteGitTransactionFixture) requireLocalState(t *testing.T, expected *completeRepositoryState) {
	t.Helper()
	actual := f.captureState(t)
	require.Equal(t, expected.localRefs, actual.localRefs)
	require.Equal(t, expected.head, actual.head)
	require.Equal(t, expected.symbolic, actual.symbolic)
	require.Equal(t, expected.status, actual.status)
	require.Equal(t, expected.index, actual.index)
	require.Equal(t, expected.config, actual.config)
	require.Equal(t, expected.locks, actual.locks)
	require.Equal(t, expected.files, actual.files)
}

func (f *remoteGitTransactionFixture) captureLocks(t *testing.T) map[string]fileState {
	t.Helper()
	root, err := os.OpenRoot(filepath.Join(f.root, ".git"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, root.Close()) })
	locks := make(map[string]fileState)
	err = fs.WalkDir(root.FS(), ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".lock") {
			return nil
		}
		data, err := fs.ReadFile(root.FS(), path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		locks[filepath.Join(".git", path)] = fileState{data: data, mode: info.Mode().Perm()}
		return nil
	})
	require.NoError(t, err)
	return locks
}

func (f *remoteGitTransactionFixture) moveRemoteBranchConcurrently(t *testing.T) string {
	t.Helper()
	clone := filepath.Join(t.TempDir(), "concurrent")
	runGit(t, t.TempDir(), "clone", f.remote, clone)
	runGit(t, clone, "checkout", "-B", "concurrent", "origin/add-image/rclone")
	require.NoError(t, os.WriteFile(filepath.Join(clone, "concurrent.txt"), []byte("remote move\n"), 0o600))
	runGit(t, clone, "add", "--", "concurrent.txt")
	runGit(t, clone, "-c", "user.name=Concurrent", "-c", "user.email=concurrent@example.com", "commit", "-m", "concurrent move")
	runGit(t, clone, "push", "origin", "HEAD:"+f.branchRef)
	return strings.TrimSpace(string(runGit(t, clone, "rev-parse", "HEAD")))
}

func (f *remoteGitTransactionFixture) remoteBranchOID(t *testing.T) string {
	t.Helper()
	return strings.TrimSpace(string(runGit(t, f.remote, "rev-parse", f.branchRef)))
}

func optionalSymbolicRef(t *testing.T, root string) string {
	t.Helper()
	value, exists := optionalGitOutput(t, root, "symbolic-ref", "-q", "HEAD")
	require.True(t, exists)
	return strings.TrimSpace(value)
}

func captureFixtureFiles(t *testing.T, root string) map[string]fileState {
	t.Helper()
	paths := []string{"copa-config.yaml", "tracked.txt"}
	sort.Strings(paths)
	files := make(map[string]fileState, len(paths))
	for _, relative := range paths {
		data, err := os.ReadFile(filepath.Join(root, relative))
		require.NoError(t, err)
		info, err := os.Stat(filepath.Join(root, relative))
		require.NoError(t, err)
		files[relative] = fileState{data: data, mode: info.Mode().Perm()}
	}
	return files
}
