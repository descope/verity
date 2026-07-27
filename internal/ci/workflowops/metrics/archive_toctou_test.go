package metrics

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/ci/workflowops/retry"
)

func TestArchiver_rejects_metrics_mutated_after_validation_before_archive(t *testing.T) {
	// Given: a valid metrics file and a runner that mutates it during origin resolution.
	gitPath, err := exec.LookPath("git")
	require.NoError(t, err)
	tmp := t.TempDir()
	remote := filepath.Join(tmp, "origin.git")
	repo := filepath.Join(tmp, "repo")
	metricsDir := filepath.Join(tmp, "metrics")
	metricsPath := filepath.Join(metricsDir, "metrics-example.json")
	runGit(t, gitPath, "", "init", "--bare", remote)
	runGit(t, gitPath, "", "init", "-b", "main", repo)
	configureGit(t, gitPath, repo)
	writeArchiveFixture(t, filepath.Join(repo, "README.md"), "seed\n")
	runGit(t, gitPath, repo, "add", "README.md")
	runGit(t, gitPath, repo, "commit", "-m", "seed")
	runGit(t, gitPath, repo, "remote", "add", "origin", remote)
	runGit(t, gitPath, repo, "push", "-u", "origin", "main")
	writeArchiveFixture(t, metricsPath, validMetricsJSON)
	expected, err := NewExpectedRun(42, 3)
	require.NoError(t, err)
	runner := &mutatingMetricsRunner{delegate: retry.ExecRunner{}, path: metricsPath}
	archiver := Archiver{Runner: runner}

	// When: the source changes after validation but before archive copying.
	_, err = archiver.Archive(t.Context(), &ArchiveOptions{
		RepoDir: repo, MetricsDir: metricsDir, Run: expected,
		RunCreatedAt: time.Date(2026, 7, 17, 6, 0, 0, 0, time.UTC),
		Attempts:     1, CommandTimeout: 5 * time.Second,
	})

	// Then: the mutation fails closed and no archive branch is published.
	require.ErrorIs(t, err, ErrInvalidMetrics)
	require.True(t, runner.mutated)
	command := exec.CommandContext(t.Context(), gitPath, "--git-dir", remote, "show-ref", "--verify", "refs/heads/_metrics")
	require.Error(t, command.Run())
}

type mutatingMetricsRunner struct {
	delegate retry.Runner
	path     string
	mutated  bool
}

func (runner *mutatingMetricsRunner) Run(ctx context.Context, command *retry.Command) (retry.Result, error) {
	if !runner.mutated && command.Name == "git" && len(command.Args) >= 2 && command.Args[0] == "remote" && command.Args[1] == "get-url" {
		if err := os.WriteFile(runner.path, []byte(`{"tampered":true}`), 0o644); err != nil {
			return retry.Result{}, err
		}
		runner.mutated = true
	}
	return runner.delegate.Run(ctx, command)
}
