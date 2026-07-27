package metrics

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/verity-org/verity/internal/ci/workflowops/retry"
)

type gitCommand struct {
	dir     string
	timeout time.Duration
	args    []string
	label   string
}

type archiveAttempt struct {
	options   *ArchiveOptions
	remote    string
	workspace string
	attempt   int
}

func (archiver *Archiver) prepareAttempt(ctx context.Context, request *archiveAttempt) error {
	if _, err := archiver.runGit(ctx, &gitCommand{
		dir: filepath.Dir(request.workspace), timeout: request.options.CommandTimeout,
		args: []string{"clone", "--quiet", "--no-checkout", request.remote, request.workspace}, label: "clone origin",
	}); err != nil {
		return err
	}

	_, branchErr := archiver.runGit(ctx, &gitCommand{
		dir: request.workspace, timeout: request.options.CommandTimeout,
		args: []string{"rev-parse", "--verify", "--quiet", "refs/remotes/origin/_metrics"}, label: "locate metrics branch",
	})
	if branchErr == nil {
		_, err := archiver.runGit(ctx, &gitCommand{
			dir: request.workspace, timeout: request.options.CommandTimeout,
			args: []string{"switch", "--detach", "refs/remotes/origin/_metrics"}, label: "checkout metrics branch",
		})
		return err
	}
	if !exitCodeIs(branchErr, 1) && !exitCodeIs(branchErr, 128) {
		return branchErr
	}

	branch := fmt.Sprintf("metrics-bootstrap-%d-%d-%d", request.options.Run.ID(), request.options.Run.Attempt(), request.attempt)
	if _, err := archiver.runGit(ctx, &gitCommand{
		dir: request.workspace, timeout: request.options.CommandTimeout,
		args: []string{"switch", "--orphan", branch}, label: "bootstrap metrics branch",
	}); err != nil {
		return err
	}
	_, err := archiver.runGit(ctx, &gitCommand{
		dir: request.workspace, timeout: request.options.CommandTimeout,
		args: []string{"rm", "-rf", "--ignore-unmatch", "."}, label: "clear bootstrap branch",
	})
	return err
}

func (archiver *Archiver) runGit(ctx context.Context, command *gitCommand) (retry.Result, error) {
	request := retry.Command{
		Name: "git", Args: command.args, Dir: command.dir, Timeout: command.timeout,
	}
	result, err := archiver.Runner.Run(ctx, &request)
	if err != nil {
		return result, fmt.Errorf("%s: %w", command.label, err)
	}
	return result, nil
}

func exitCodeIs(err error, code int) bool {
	var commandErr *retry.CommandError
	return errors.As(err, &commandErr) && commandErr.ExitCode == code
}
