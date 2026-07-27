package repositoryops

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
)

var ErrMissingGitHubToken = errors.New("GITHUB_TOKEN is required for repository mutation")

type GitCommand struct {
	Args  []string
	Dir   string
	Stdin []byte
}

type GitRunner interface {
	Run(context.Context, GitCommand) (CommandResult, error)
}

type execGitRunner struct {
	commands CommandRunner
}

func NewGitRunner(commands CommandRunner) GitRunner {
	if commands == nil {
		commands = ExecCommandRunner{}
	}
	return &execGitRunner{commands: commands}
}

func (r *execGitRunner) Run(ctx context.Context, request GitCommand) (CommandResult, error) {
	command := &Command{Name: "git", Args: request.Args, Dir: request.Dir}
	if len(request.Stdin) > 0 {
		command.Stdin = bytes.NewReader(request.Stdin)
	}
	return r.commands.Run(ctx, command)
}

type GitHubCommand struct {
	Args []string
	Dir  string
}

type GitHubRunner interface {
	Run(context.Context, GitHubCommand) (CommandResult, error)
}

type execGitHubRunner struct {
	commands CommandRunner
	token    string
}

func NewGitHubRunner(commands CommandRunner, githubToken string) (GitHubRunner, error) {
	if strings.TrimSpace(githubToken) == "" {
		return nil, ErrMissingGitHubToken
	}
	if containsControl(githubToken) {
		return nil, fmt.Errorf("%w: token contains control characters", ErrMissingGitHubToken)
	}
	if commands == nil {
		commands = ExecCommandRunner{}
	}
	return &execGitHubRunner{commands: commands, token: githubToken}, nil
}

func (r *execGitHubRunner) Run(ctx context.Context, request GitHubCommand) (CommandResult, error) {
	return r.commands.Run(ctx, &Command{
		Name: "gh",
		Args: request.Args,
		Dir:  request.Dir,
		Env:  []string{"GH_TOKEN=" + r.token},
	})
}
