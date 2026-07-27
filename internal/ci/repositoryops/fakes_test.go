package repositoryops_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ops "github.com/verity-org/verity/internal/ci/repositoryops"
)

var errUnexpectedFakeCommand = errors.New("unexpected fake command")

const expectedGitHubRepository = "verity-org/verity"

const fakeGitHeadOID = "1111111111111111111111111111111111111111"

type fakeGitMetadata struct {
	root string
}

func newFakeGitMetadata(t *testing.T, root string) *fakeGitMetadata {
	t.Helper()
	gitDir := filepath.Join(root, ".git")
	requireNoError(t, os.MkdirAll(gitDir, 0o700))
	requireNoError(t, os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/main\n"), 0o600))
	requireNoError(t, os.WriteFile(filepath.Join(gitDir, "index"), []byte("fake-index\n"), 0o600))
	requireNoError(t, os.WriteFile(filepath.Join(gitDir, "config"), []byte("[core]\n\trepositoryformatversion = 0\n"), 0o600))
	return &fakeGitMetadata{root: root}
}

func (m *fakeGitMetadata) response(command ops.GitCommand) (ops.CommandResult, bool) {
	joined := strings.Join(command.Args, " ")
	switch joined {
	case "rev-parse --verify HEAD":
		return ops.CommandResult{Stdout: []byte(fakeGitHeadOID + "\n")}, true
	case "symbolic-ref -q HEAD":
		return ops.CommandResult{Stdout: []byte("refs/heads/main\n")}, true
	case "rev-parse --verify --quiet refs/heads/add-image/rclone":
		return ops.CommandResult{ExitCode: 1}, true
	case "rev-parse --verify --quiet refs/remotes/origin/add-image/rclone":
		return ops.CommandResult{ExitCode: 1}, true
	case "ls-remote --refs origin refs/heads/add-image/rclone":
		return ops.CommandResult{}, true
	case "rev-parse --path-format=absolute --git-dir":
		return ops.CommandResult{Stdout: []byte(filepath.Join(m.root, ".git") + "\n")}, true
	case "rev-parse --path-format=absolute --git-common-dir":
		return ops.CommandResult{Stdout: []byte(filepath.Join(m.root, ".git") + "\n")}, true
	case "rev-parse --path-format=absolute --git-path HEAD":
		return ops.CommandResult{Stdout: []byte(filepath.Join(m.root, ".git", "HEAD") + "\n")}, true
	case "rev-parse --path-format=absolute --git-path index":
		return ops.CommandResult{Stdout: []byte(filepath.Join(m.root, ".git", "index") + "\n")}, true
	case "rev-parse --path-format=absolute --git-path config":
		return ops.CommandResult{Stdout: []byte(filepath.Join(m.root, ".git", "config") + "\n")}, true
	case "update-ref --stdin":
		return ops.CommandResult{}, true
	default:
		return ops.CommandResult{}, false
	}
}

func requireNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

type fakeCommandRunner struct {
	calls     []ops.Command
	contexts  []context.Context
	responses []ops.CommandResult
	errors    []error
	run       func(context.Context, ops.Command, int) (ops.CommandResult, error)
}

func (f *fakeCommandRunner) Run(ctx context.Context, command *ops.Command) (ops.CommandResult, error) {
	callIndex := len(f.calls)
	call := *command
	f.calls = append(f.calls, call)
	f.contexts = append(f.contexts, ctx)
	if f.run != nil {
		return f.run(ctx, call, callIndex)
	}
	if callIndex >= len(f.responses) {
		return ops.CommandResult{}, fmt.Errorf("%w: %s %v", errUnexpectedFakeCommand, command.Name, command.Args)
	}
	var err error
	if callIndex < len(f.errors) {
		err = f.errors[callIndex]
	}
	return f.responses[callIndex], err
}

type fakeGitRunner struct {
	calls []ops.GitCommand
	run   func(context.Context, ops.GitCommand, int) (ops.CommandResult, error)
}

func (f *fakeGitRunner) Run(ctx context.Context, command ops.GitCommand) (ops.CommandResult, error) {
	callIndex := len(f.calls)
	f.calls = append(f.calls, command)
	if f.run == nil {
		return ops.CommandResult{}, fmt.Errorf("%w: git %v", errUnexpectedFakeCommand, command.Args)
	}
	return f.run(ctx, command, callIndex)
}

type fakeGitHubRunner struct {
	calls []ops.GitHubCommand
	run   func(context.Context, ops.GitHubCommand, int) (ops.CommandResult, error)
}

func (f *fakeGitHubRunner) Run(ctx context.Context, command ops.GitHubCommand) (ops.CommandResult, error) {
	callIndex := len(f.calls)
	f.calls = append(f.calls, command)
	if f.run == nil {
		return ops.CommandResult{}, fmt.Errorf("%w: GitHub %v", errUnexpectedFakeCommand, command.Args)
	}
	return f.run(ctx, command, callIndex)
}

type fakePatcher struct {
	specs  []ops.PatchSpec
	errors []error
}

func (f *fakePatcher) Patch(_ context.Context, spec *ops.PatchSpec) error {
	callIndex := len(f.specs)
	f.specs = append(f.specs, *spec)
	if callIndex >= len(f.errors) {
		return nil
	}
	return f.errors[callIndex]
}
