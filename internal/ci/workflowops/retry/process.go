package retry

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"
)

var (
	ErrCommandNameRequired = errors.New("run command: name is required")
	ErrOperationRequired   = errors.New("retry operation is required")
	ErrRunnerRequired      = errors.New("command runner is required")
)

type Command struct {
	Name    string
	Args    []string
	Dir     string
	Stdin   []byte
	Timeout time.Duration
}

type Result struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

type CommandError struct {
	ExitCode int
	TimedOut bool
	Cause    error
}

func (err *CommandError) Error() string {
	if err.TimedOut {
		return "command timed out"
	}
	return fmt.Sprintf("command exited with code %d", err.ExitCode)
}

func (err *CommandError) Unwrap() error {
	return err.Cause
}

type Runner interface {
	Run(context.Context, *Command) (Result, error)
}

type Operation struct {
	Label   string
	Command Command
}

type Process struct {
	Runner Runner
	Engine Engine
	Stdout io.Writer
	Stderr io.Writer
}

func (process *Process) Run(ctx context.Context, operation *Operation, policy Policy) error {
	if operation == nil {
		return ErrOperationRequired
	}
	if process.Runner == nil {
		return fmt.Errorf("run %q: %w", operation.Label, ErrRunnerRequired)
	}
	stderr := process.Stderr
	if stderr == nil {
		stderr = io.Discard
	}
	stdout := process.Stdout
	if stdout == nil {
		stdout = io.Discard
	}

	engine := process.Engine
	previousObserver := engine.Observe
	engine.Observe = func(event Event) error {
		var commandErr *CommandError
		if errors.As(event.Err, &commandErr) {
			if _, err := fmt.Fprintf(stderr, "%s failed (exit %d); retrying in %s...\n", operation.Label, commandErr.ExitCode, event.Delay); err != nil {
				return fmt.Errorf("write retry notice: %w", err)
			}
		} else {
			if _, err := fmt.Fprintf(stderr, "%s failed; retrying in %s...\n", operation.Label, event.Delay); err != nil {
				return fmt.Errorf("write retry notice: %w", err)
			}
		}
		if previousObserver != nil {
			return previousObserver(event)
		}
		return nil
	}

	return engine.Do(ctx, policy, func(parent context.Context, attempt int) error {
		attemptCtx := parent
		cancel := func() {}
		if operation.Command.Timeout > 0 {
			attemptCtx, cancel = context.WithTimeout(parent, operation.Command.Timeout)
		}
		defer cancel()

		if _, err := fmt.Fprintf(stderr, "::group::%s (attempt %d/%d)\n", operation.Label, attempt, policy.MaxAttempts); err != nil {
			return fmt.Errorf("write retry group: %w", err)
		}
		result, runErr := process.Runner.Run(attemptCtx, &operation.Command)
		if _, err := stdout.Write(result.Stdout); err != nil {
			return fmt.Errorf("write command stdout: %w", err)
		}
		if _, err := stderr.Write(result.Stderr); err != nil {
			return fmt.Errorf("write command stderr: %w", err)
		}
		if _, err := fmt.Fprintln(stderr, "::endgroup::"); err != nil {
			return fmt.Errorf("write retry group end: %w", err)
		}
		if runErr != nil {
			return fmt.Errorf("%s attempt %d: %w", operation.Label, attempt, runErr)
		}
		return nil
	})
}
