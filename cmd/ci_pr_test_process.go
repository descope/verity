package cmd

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

var errPRCommandFailed = errors.New("PR test command failed")

type prCommandRequest struct {
	Name      string
	Args      []string
	Dir       string
	Env       []string
	Stdin     io.Reader
	Stdout    io.Writer
	Stderr    io.Writer
	Timeout   time.Duration
	TermGrace time.Duration
}

type prCommandResult struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

type prCommandError struct {
	Name     string
	ExitCode int
	Details  string
}

func (e *prCommandError) Error() string {
	if e.Details == "" {
		return fmt.Sprintf("%s exited %d", e.Name, e.ExitCode)
	}
	return fmt.Sprintf("%s exited %d: %s", e.Name, e.ExitCode, e.Details)
}

func (e *prCommandError) Unwrap() error {
	return errPRCommandFailed
}

func runPRCommand(ctx context.Context, request *prCommandRequest) (prCommandResult, error) {
	if request == nil {
		return prCommandResult{}, fmt.Errorf("%w: command request is required", errPRCommandFailed)
	}
	if strings.TrimSpace(request.Name) == "" {
		return prCommandResult{}, fmt.Errorf("%w: command name is required", errPRCommandFailed)
	}
	if request.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, request.Timeout)
		defer cancel()
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
		if err := syscall.Kill(-command.Process.Pid, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
			return fmt.Errorf("terminate %s process group: %w", request.Name, err)
		}
		return nil
	}
	command.WaitDelay = request.TermGrace
	if command.WaitDelay == 0 {
		command.WaitDelay = time.Minute
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if request.Stdout != nil {
		command.Stdout = io.MultiWriter(request.Stdout, &stdout)
	}
	if request.Stderr != nil {
		command.Stderr = io.MultiWriter(request.Stderr, &stderr)
	}

	err := command.Run()
	result := prCommandResult{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}
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
	return prCommandResult{}, fmt.Errorf("start %s: %w", request.Name, err)
}

func requirePRCommand(ctx context.Context, request *prCommandRequest) (prCommandResult, error) {
	result, err := runPRCommand(ctx, request)
	if err != nil {
		return result, err
	}
	if result.ExitCode == 0 {
		return result, nil
	}
	details := strings.TrimSpace(string(append(append([]byte(nil), result.Stdout...), result.Stderr...)))
	if len(details) > 4096 {
		details = details[:4096]
	}
	return result, &prCommandError{Name: request.Name, ExitCode: result.ExitCode, Details: details}
}

func appendPRGitHubValues(path string, values [][2]string) (err error) {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("%w: GitHub output path is required", errPRCommandFailed)
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open GitHub output %q: %w", path, err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("close GitHub output %q: %w", path, closeErr)
		}
	}()
	for _, value := range values {
		if strings.ContainsAny(value[0], "=\r\n") || strings.ContainsAny(value[1], "\r\n") {
			return fmt.Errorf("%w: invalid GitHub output %q", errPRCommandFailed, value[0])
		}
		if _, err := fmt.Fprintf(file, "%s=%s\n", value[0], value[1]); err != nil {
			return fmt.Errorf("write GitHub output %q: %w", value[0], err)
		}
	}
	return nil
}
