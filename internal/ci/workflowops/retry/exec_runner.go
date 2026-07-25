package retry

import (
	"bytes"
	"context"
	cryptorand "crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"os/exec"
	"time"
)

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, command *Command) (Result, error) {
	if command == nil || command.Name == "" {
		return Result{}, ErrCommandNameRequired
	}

	process := exec.CommandContext(ctx, command.Name, command.Args...)
	process.Dir = command.Dir
	process.Stdin = bytes.NewReader(command.Stdin)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	process.Stdout = &stdout
	process.Stderr = &stderr

	runErr := process.Run()
	result := Result{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}
	if runErr == nil {
		return result, nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		result.ExitCode = 124
		return result, &CommandError{
			ExitCode: result.ExitCode,
			TimedOut: errors.Is(ctxErr, context.DeadlineExceeded),
			Cause:    ctxErr,
		}
	}

	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
		return result, &CommandError{ExitCode: result.ExitCode, Cause: runErr}
	}
	result.ExitCode = -1
	return result, &CommandError{ExitCode: result.ExitCode, Cause: runErr}
}

type TimerSleeper struct{}

func (TimerSleeper) Wait(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

type SystemRandom struct{}

func (SystemRandom) Intn(limit int) (int, error) {
	value, err := cryptorand.Int(cryptorand.Reader, big.NewInt(int64(limit)))
	if err != nil {
		return 0, fmt.Errorf("read cryptographic random source: %w", err)
	}
	return int(value.Int64()), nil
}
