package repositoryops

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type rollbackGitFixture struct {
	root          string
	remote        string
	branch        string
	preservedLock string
}

type rollbackPathState struct {
	Mode       fs.FileMode
	Data       string
	LinkTarget string
}

type rollbackCapture struct {
	root      string
	locksOnly bool
	states    map[string]rollbackPathState
}

type rollbackRepositoryState struct {
	Head       string
	Symbolic   string
	Status     string
	LocalRefs  string
	RemoteRefs string
	HeadFile   string
	ConfigFile string
	Worktree   map[string]rollbackPathState
	Locks      map[string]rollbackPathState
}

func newRollbackGitFixture(t *testing.T) rollbackGitFixture {
	t.Helper()
	base := t.TempDir()
	root := filepath.Join(base, "work")
	remote := filepath.Join(base, "origin.git")
	require.NoError(t, os.Mkdir(root, 0o700))
	runRepositoryGit(t, base, "init", "--bare", remote)
	runRepositoryGit(t, root, "init", "-b", "main")
	require.NoError(t, os.Mkdir(filepath.Join(root, "nested"), 0o750))
	require.NoError(t, os.Chmod(filepath.Join(root, "nested"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(root, "tracked.txt"), []byte("committed\n"), 0o640))
	require.NoError(t, os.WriteFile(filepath.Join(root, "nested", "original.txt"), []byte("nested\n"), 0o600))
	require.NoError(t, os.Symlink("tracked.txt", filepath.Join(root, "tracked-link")))
	runRepositoryGit(t, root, "add", "--", "tracked.txt", "nested/original.txt", "tracked-link")
	runRepositoryGit(t, root, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.com", "commit", "-m", "baseline")
	runRepositoryGit(t, root, "remote", "add", "origin", remote)
	runRepositoryGit(t, root, "push", "-u", "origin", "main")
	require.NoError(t, os.WriteFile(filepath.Join(root, "tracked.txt"), []byte("staged-before\n"), 0o600))
	require.NoError(t, os.Chmod(filepath.Join(root, "tracked.txt"), 0o600))
	runRepositoryGit(t, root, "add", "--", "tracked.txt")
	require.NoError(t, os.WriteFile(filepath.Join(root, "tracked.txt"), []byte("unstaged-before\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "untracked.txt"), []byte("untracked-before\n"), 0o660))
	preservedLock := filepath.Join(root, ".git", "preserved.lock")
	require.NoError(t, os.WriteFile(preservedLock, []byte("preserved-lock\n"), 0o600))
	return rollbackGitFixture{root: root, remote: remote, branch: "add-image/sentinel", preservedLock: preservedLock}
}

func captureRollbackRepositoryState(t *testing.T, fixture rollbackGitFixture) rollbackRepositoryState {
	t.Helper()
	gitDirectory := filepath.Join(fixture.root, ".git")
	return rollbackRepositoryState{
		Head:       strings.TrimSpace(runRepositoryGit(t, fixture.root, "rev-parse", "HEAD")),
		Symbolic:   strings.TrimSpace(runRepositoryGit(t, fixture.root, "symbolic-ref", "HEAD")),
		Status:     runRepositoryGit(t, fixture.root, "status", "--porcelain=v1", "-z", "--untracked-files=all"),
		LocalRefs:  runRepositoryGit(t, fixture.root, "for-each-ref", "--format=%(refname) %(objectname)"),
		RemoteRefs: runRepositoryGit(t, fixture.remote, "for-each-ref", "--format=%(refname) %(objectname)"),
		HeadFile:   readRollbackFile(t, filepath.Join(gitDirectory, "HEAD")),
		ConfigFile: readRollbackFile(t, filepath.Join(gitDirectory, "config")),
		Worktree:   captureRollbackPaths(t, fixture.root, false),
		Locks:      captureRollbackPaths(t, gitDirectory, true),
	}
}

func captureRollbackPaths(t *testing.T, root string, locksOnly bool) map[string]rollbackPathState {
	t.Helper()
	capture := rollbackCapture{root: root, locksOnly: locksOnly, states: make(map[string]rollbackPathState)}
	err := capture.directory("")
	require.NoError(t, err)
	return capture.states
}

func (capture *rollbackCapture) directory(relative string) error {
	directory := capture.root
	if relative != "" {
		directory = filepath.Join(capture.root, relative)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		childRelative := filepath.Join(relative, entry.Name())
		if !capture.locksOnly && childRelative == ".git" && entry.IsDir() {
			continue
		}
		if entry.IsDir() {
			if err := capture.directory(childRelative); err != nil {
				return err
			}
			continue
		}
		if capture.locksOnly && !strings.HasSuffix(entry.Name(), ".lock") {
			continue
		}
		path := filepath.Join(capture.root, childRelative)
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		state := rollbackPathState{Mode: info.Mode()}
		if info.Mode()&fs.ModeSymlink != 0 {
			state.LinkTarget, err = os.Readlink(path)
		} else if info.Mode().IsRegular() {
			var content []byte
			content, err = os.ReadFile(path)
			state.Data = string(content)
		}
		if err != nil {
			return err
		}
		capture.states[filepath.ToSlash(childRelative)] = state
	}
	return nil
}

func readRollbackFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(content)
}

func runRepositoryGit(t *testing.T, directory string, args ...string) string {
	t.Helper()
	command := exec.CommandContext(t.Context(), "git", args...)
	command.Dir = directory
	output, err := command.CombinedOutput()
	require.NoError(t, err, string(output))
	return string(output)
}
