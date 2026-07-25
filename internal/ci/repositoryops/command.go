package repositoryops

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

var (
	ErrCommandFailed        = errors.New("repository operation command failed")
	ErrDependenciesRequired = errors.New("repository operation dependencies are required")
)

type Command struct {
	Name   string
	Args   []string
	Dir    string
	Env    []string
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

type CommandResult struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

type CommandRunner interface {
	Run(context.Context, *Command) (CommandResult, error)
}

type ExecCommandRunner struct{}

func (ExecCommandRunner) Run(ctx context.Context, request *Command) (CommandResult, error) {
	if request == nil {
		return CommandResult{}, fmt.Errorf("%w: command request is required", ErrCommandFailed)
	}
	if err := ctx.Err(); err != nil {
		return CommandResult{}, fmt.Errorf("run %s: %w", request.Name, err)
	}
	if strings.TrimSpace(request.Name) == "" {
		return CommandResult{}, fmt.Errorf("%w: command name is required", ErrCommandFailed)
	}

	command := exec.CommandContext(ctx, request.Name, request.Args...)
	command.Dir = request.Dir
	command.Env = append(os.Environ(), request.Env...)
	command.Stdin = request.Stdin
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error {
		if command.Process == nil {
			return nil
		}
		if err := syscall.Kill(-command.Process.Pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
			return fmt.Errorf("kill %s process group: %w", request.Name, err)
		}
		return nil
	}
	command.WaitDelay = 2 * time.Second

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if request.Stdout == nil {
		command.Stdout = &stdout
	} else {
		command.Stdout = request.Stdout
	}
	if request.Stderr == nil {
		command.Stderr = &stderr
	} else {
		command.Stderr = request.Stderr
	}

	err := command.Run()
	result := CommandResult{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}
	if err == nil {
		return result, nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return result, fmt.Errorf("run %s: %w", request.Name, ctxErr)
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		result.ExitCode = exitError.ExitCode()
		return result, nil
	}
	return CommandResult{}, fmt.Errorf("start %s: %w", request.Name, err)
}

type CommandError struct {
	Command  string
	ExitCode int
	Details  string
}

func (e *CommandError) Error() string {
	if e.Details == "" {
		return fmt.Sprintf("%s exited %d", e.Command, e.ExitCode)
	}
	return fmt.Sprintf("%s exited %d: %s", e.Command, e.ExitCode, e.Details)
}

func (e *CommandError) Unwrap() error {
	return ErrCommandFailed
}

func runRequiredCommand(ctx context.Context, runner CommandRunner, request *Command) (CommandResult, error) {
	if runner == nil {
		return CommandResult{}, fmt.Errorf("%w: command runner is required", ErrCommandFailed)
	}
	result, err := runner.Run(ctx, request)
	if err != nil {
		return CommandResult{}, fmt.Errorf("run %s: %w", request.Name, err)
	}
	if result.ExitCode != 0 {
		return CommandResult{}, commandError(request.Name, result)
	}
	return result, nil
}

func commandError(name string, result CommandResult) error {
	details := strings.TrimSpace(string(append(append([]byte(nil), result.Stdout...), result.Stderr...)))
	if len(details) > 4096 {
		details = details[:4096]
	}
	return &CommandError{Command: name, ExitCode: result.ExitCode, Details: details}
}
