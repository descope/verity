package metrics

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/verity-org/verity/internal/ci/workflowops/retry"
)

var ErrInvalidArchive = errors.New("invalid metrics archive request")

type ArchiveOptions struct {
	RepoDir        string
	MetricsDir     string
	Run            ExpectedRun
	RunCreatedAt   time.Time
	Attempts       int
	RetryDelay     time.Duration
	RetryJitter    time.Duration
	CommandTimeout time.Duration
}

type ArchiveResult struct {
	NoChanges bool
	Attempts  int
	TargetDir string
}

type Archiver struct {
	Runner retry.Runner
	Engine retry.Engine
	Stdout io.Writer
}

func (archiver *Archiver) Archive(ctx context.Context, options *ArchiveOptions) (result ArchiveResult, retErr error) {
	if err := validateArchiveOptions(options); err != nil {
		return ArchiveResult{}, err
	}
	if archiver.Runner == nil {
		return ArchiveResult{}, fmt.Errorf("%w: git runner is required", ErrInvalidArchive)
	}

	validation, err := ValidateDirectory(ctx, options.Run, options.MetricsDir)
	if err != nil {
		return ArchiveResult{}, fmt.Errorf("validate metrics before archive: %w", err)
	}
	remoteResult, err := archiver.runGit(ctx, &gitCommand{
		dir: options.RepoDir, timeout: options.CommandTimeout,
		args: []string{"remote", "get-url", "origin"}, label: "resolve origin",
	})
	if err != nil {
		return ArchiveResult{}, err
	}
	remote := strings.TrimSpace(string(remoteResult.Stdout))
	if remote == "" {
		return ArchiveResult{}, fmt.Errorf("%w: origin URL is empty", ErrInvalidArchive)
	}

	tempRoot, err := os.MkdirTemp("", "verity-metrics-archive-")
	if err != nil {
		return ArchiveResult{}, fmt.Errorf("create archive workspace: %w", err)
	}
	defer func() {
		if cleanupErr := os.RemoveAll(tempRoot); cleanupErr != nil {
			retErr = errors.Join(retErr, fmt.Errorf("remove archive workspace: %w", cleanupErr))
		}
	}()

	targetDir := filepath.ToSlash(filepath.Join(
		"_metrics", "runs", options.RunCreatedAt.UTC().Format("2006-01-02"),
		fmt.Sprintf("%d-attempt-%d", options.Run.ID(), options.Run.Attempt()),
	))
	result.TargetDir = targetDir
	policy := retry.Policy{MaxAttempts: options.Attempts, BaseDelay: options.RetryDelay, Jitter: options.RetryJitter}
	run := archiveRun{
		archiver: archiver, options: options, artifacts: validation.Artifacts,
		remote: remote, tempRoot: tempRoot, targetDir: targetDir, result: &result,
	}
	err = archiver.Engine.Do(ctx, policy, run.attempt)
	if err != nil {
		return ArchiveResult{}, fmt.Errorf("archive metrics after %d attempt(s): %w", result.Attempts, err)
	}

	if err := archiver.writeResult(result); err != nil {
		return ArchiveResult{}, err
	}
	return result, nil
}

type archiveRun struct {
	archiver  *Archiver
	options   *ArchiveOptions
	artifacts []ValidatedArtifact
	remote    string
	tempRoot  string
	targetDir string
	result    *ArchiveResult
}

func (run *archiveRun) attempt(ctx context.Context, attempt int) error {
	run.result.Attempts = attempt
	workspace := filepath.Join(run.tempRoot, fmt.Sprintf("attempt-%d", attempt))
	request := archiveAttempt{options: run.options, remote: run.remote, workspace: workspace, attempt: attempt}
	if err := run.archiver.prepareAttempt(ctx, &request); err != nil {
		return err
	}
	if err := copyArchivedMetrics(run.artifacts, workspace, run.targetDir); err != nil {
		return err
	}
	if _, err := run.archiver.runGit(ctx, &gitCommand{
		dir: workspace, timeout: run.options.CommandTimeout,
		args: []string{"add", "_metrics/"}, label: "stage metrics",
	}); err != nil {
		return err
	}
	return run.commitAndPush(ctx, workspace)
}

func (run *archiveRun) commitAndPush(ctx context.Context, workspace string) error {
	diffResult, diffErr := run.archiver.runGit(ctx, &gitCommand{
		dir: workspace, timeout: run.options.CommandTimeout,
		args: []string{"diff", "--cached", "--quiet"}, label: "inspect metrics diff",
	})
	if diffErr == nil {
		run.result.NoChanges = true
		return nil
	}
	if !exitCodeIs(diffErr, 1) {
		return fmt.Errorf("inspect staged metrics (exit %d): %w", diffResult.ExitCode, diffErr)
	}
	message := fmt.Sprintf("metrics: run %d attempt %d", run.options.Run.ID(), run.options.Run.Attempt())
	countMessage := fmt.Sprintf("%d image(s)", len(run.artifacts))
	if _, err := run.archiver.runGit(ctx, &gitCommand{
		dir: workspace, timeout: run.options.CommandTimeout,
		args:  []string{"-c", "user.name=verity-ci", "-c", "user.email=ci@verity.invalid", "commit", "-m", message, "-m", countMessage},
		label: "commit metrics",
	}); err != nil {
		return err
	}
	_, err := run.archiver.runGit(ctx, &gitCommand{
		dir: workspace, timeout: run.options.CommandTimeout,
		args: []string{"push", "origin", "HEAD:refs/heads/_metrics"}, label: "push metrics",
	})
	return err
}

func (archiver *Archiver) writeResult(result ArchiveResult) error {
	if result.NoChanges {
		return writeArchiveNotice(archiver.Stdout, "::notice::No changes to commit\n")
	}
	return writeArchiveNotice(archiver.Stdout, fmt.Sprintf("::notice::Pushed on attempt %d\n", result.Attempts))
}

func validateArchiveOptions(options *ArchiveOptions) error {
	if options == nil || options.RepoDir == "" || options.MetricsDir == "" || options.RunCreatedAt.IsZero() {
		return fmt.Errorf("%w: repository, metrics directory, and run creation time are required", ErrInvalidArchive)
	}
	if options.Attempts < 1 || options.CommandTimeout <= 0 || options.RetryDelay < 0 || options.RetryJitter < 0 {
		return fmt.Errorf("%w: retry and timeout values are invalid", ErrInvalidArchive)
	}
	return nil
}
