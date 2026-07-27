package metrics

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/ci/workflowops/retry"
)

func TestArchiver_publishes_metrics_without_touching_dirty_worktree(t *testing.T) {
	// Given: a dirty repository, a bare origin, and a validated metrics artifact.
	gitPath, err := exec.LookPath("git")
	require.NoError(t, err)
	tmp := t.TempDir()
	remote := filepath.Join(tmp, "origin.git")
	repo := filepath.Join(tmp, "repo")
	metricsDir := filepath.Join(tmp, "metrics")
	runGit(t, gitPath, "", "init", "--bare", remote)
	runGit(t, gitPath, "", "init", "-b", "main", repo)
	configureGit(t, gitPath, repo)
	writeArchiveFixture(t, filepath.Join(repo, "README.md"), "clean\n")
	runGit(t, gitPath, repo, "add", "README.md")
	runGit(t, gitPath, repo, "commit", "-m", "seed")
	runGit(t, gitPath, repo, "remote", "add", "origin", remote)
	runGit(t, gitPath, repo, "push", "-u", "origin", "main")
	writeArchiveFixture(t, filepath.Join(repo, "README.md"), "dirty\n")
	writeArchiveFixture(t, filepath.Join(repo, "untracked.txt"), "preserve\n")
	writeArchiveFixture(t, filepath.Join(metricsDir, "metrics-example-1.2.3.json"), validMetricsJSON)
	beforeStatus := runGit(t, gitPath, repo, "status", "--short")
	expected, err := NewExpectedRun(42, 3)
	require.NoError(t, err)
	var output bytes.Buffer
	archiver := Archiver{Runner: retry.ExecRunner{}, Stdout: &output}

	// When: the Go archiver publishes through an isolated temporary clone.
	result, err := archiver.Archive(t.Context(), &ArchiveOptions{
		RepoDir:        repo,
		MetricsDir:     metricsDir,
		Run:            expected,
		RunCreatedAt:   time.Date(2026, 7, 17, 6, 0, 0, 0, time.UTC),
		Attempts:       1,
		CommandTimeout: 5 * time.Second,
	})

	// Then: origin receives the run while the caller's dirty state and branch remain unchanged.
	require.NoError(t, err)
	assert.False(t, result.NoChanges)
	assert.Equal(t, beforeStatus, runGit(t, gitPath, repo, "status", "--short"))
	assert.Equal(t, "main", strings.TrimSpace(runGit(t, gitPath, repo, "branch", "--show-current")))
	verify := filepath.Join(tmp, "verify")
	runGit(t, gitPath, "", "clone", "--branch", "_metrics", remote, verify)
	assert.FileExists(t, filepath.Join(verify, "_metrics", "runs", "2026-07-17", "42-attempt-3", "example-1.2.3.json"))
	assert.Contains(t, output.String(), "Pushed on attempt 1")
}

func TestArchiver_retries_push_from_a_fresh_clone(t *testing.T) {
	// Given: an origin and a runner that rejects the first metrics push.
	gitPath, err := exec.LookPath("git")
	require.NoError(t, err)
	tmp := t.TempDir()
	remote := filepath.Join(tmp, "origin.git")
	repo := filepath.Join(tmp, "repo")
	metricsDir := filepath.Join(tmp, "metrics")
	runGit(t, gitPath, "", "init", "--bare", remote)
	runGit(t, gitPath, "", "init", "-b", "main", repo)
	configureGit(t, gitPath, repo)
	writeArchiveFixture(t, filepath.Join(repo, "README.md"), "seed\n")
	runGit(t, gitPath, repo, "add", "README.md")
	runGit(t, gitPath, repo, "commit", "-m", "seed")
	runGit(t, gitPath, repo, "remote", "add", "origin", remote)
	runGit(t, gitPath, repo, "push", "-u", "origin", "main")
	writeArchiveFixture(t, filepath.Join(metricsDir, "metrics-example.json"), validMetricsJSON)
	expected, err := NewExpectedRun(42, 3)
	require.NoError(t, err)
	runner := &flakyPushRunner{delegate: retry.ExecRunner{}}
	var output bytes.Buffer
	archiver := Archiver{Runner: runner, Stdout: &output}

	// When: two attempts are allowed with zero test delay.
	result, err := archiver.Archive(t.Context(), &ArchiveOptions{
		RepoDir: repo, MetricsDir: metricsDir, Run: expected,
		RunCreatedAt: time.Date(2026, 7, 17, 6, 0, 0, 0, time.UTC),
		Attempts:     2, CommandTimeout: 5 * time.Second,
	})

	// Then: the rejected push is retried from a second isolated clone.
	require.NoError(t, err)
	assert.Equal(t, 2, result.Attempts)
	assert.Equal(t, 2, runner.pushes)
	assert.Contains(t, output.String(), "Pushed on attempt 2")
}

func configureGit(t *testing.T, gitPath, dir string) {
	t.Helper()
	runGit(t, gitPath, dir, "config", "user.name", "test")
	runGit(t, gitPath, dir, "config", "user.email", "test@example.com")
}

func runGit(t *testing.T, gitPath, dir string, args ...string) string {
	t.Helper()
	command := exec.CommandContext(t.Context(), gitPath, args...)
	command.Dir = dir
	output, err := command.CombinedOutput()
	require.NoError(t, err, "%s: %s", strings.Join(args, " "), output)
	return string(output)
}

func writeArchiveFixture(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

type flakyPushRunner struct {
	delegate retry.Runner
	pushes   int
}

func (runner *flakyPushRunner) Run(ctx context.Context, command *retry.Command) (retry.Result, error) {
	if command.Name == "git" && len(command.Args) > 0 && command.Args[0] == "push" {
		runner.pushes++
		if runner.pushes == 1 {
			return retry.Result{ExitCode: 1}, &retry.CommandError{ExitCode: 1}
		}
	}
	return runner.delegate.Run(ctx, command)
}
