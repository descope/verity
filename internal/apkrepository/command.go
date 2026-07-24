package apkrepository

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

type command struct {
	name   string
	args   []string
	dir    string
	stdout io.Writer
}

type commandResult struct {
	stdout   []byte
	stderr   []byte
	exitCode int
}

type commandRunner interface {
	Run(context.Context, command) (commandResult, error)
}

type execCommandRunner struct{}

func (execCommandRunner) Run(ctx context.Context, request command) (commandResult, error) {
	cmd := exec.CommandContext(ctx, request.name, request.args...)
	cmd.Dir = request.dir
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if request.stdout == nil {
		cmd.Stdout = &stdout
	} else {
		cmd.Stdout = request.stdout
	}
	cmd.Stderr = &stderr
	err := cmd.Run()
	result := commandResult{stdout: stdout.Bytes(), stderr: stderr.Bytes()}
	if err == nil {
		return result, nil
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		result.exitCode = exitError.ExitCode()
		return result, nil
	}
	return commandResult{}, fmt.Errorf("start %s: %w", request.name, err)
}

func runRequired(ctx context.Context, runner commandRunner, request command) (commandResult, error) {
	result, err := runner.Run(ctx, request)
	if err != nil {
		return commandResult{}, err
	}
	if result.exitCode != 0 {
		details := strings.TrimSpace(string(append(result.stdout, result.stderr...)))
		return commandResult{}, fmt.Errorf("%w: %s exited %d: %s", errCommandFailed, request.name, result.exitCode, details)
	}
	return result, nil
}
