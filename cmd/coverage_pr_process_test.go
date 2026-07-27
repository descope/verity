package cmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const coveragePRCommandHelper = "COVERAGE_PR_COMMAND_HELPER"

type coveragePRCommandOutcome struct {
	result prCommandResult
	err    error
}

type coveragePRReadyWriter struct {
	once  sync.Once
	ready chan struct{}
}

func (writer *coveragePRReadyWriter) Write(data []byte) (int, error) {
	writer.once.Do(func() { close(writer.ready) })
	return len(data), nil
}

func TestCoveragePRCommandHelperProcess(t *testing.T) {
	if os.Getenv(coveragePRCommandHelper) == "" {
		return
	}

	switch os.Getenv(coveragePRCommandHelper) {
	case "success":
		input, err := io.ReadAll(os.Stdin)
		if err != nil {
			os.Exit(98)
		}
		workingDirectory, err := os.Getwd()
		if err != nil {
			os.Exit(97)
		}
		_, _ = fmt.Fprintf(os.Stdout, "stdout|%s|%s|%s", os.Getenv("PR_SENTINEL"), filepath.Base(workingDirectory), input)
		_, _ = fmt.Fprint(os.Stderr, "stderr|sentinel")
		os.Exit(0)
	case "exit":
		_, _ = fmt.Fprint(os.Stdout, "stdout failure")
		_, _ = fmt.Fprint(os.Stderr, " stderr failure ")
		os.Exit(23)
	case "long-exit":
		_, _ = fmt.Fprint(os.Stdout, strings.Repeat("x", 5000))
		_, _ = fmt.Fprint(os.Stderr, "stderr-tail")
		os.Exit(17)
	case "fake-verity":
		arguments := coveragePRHelperArguments()
		if len(arguments) > 0 && arguments[0] == "discover" {
			_, _ = fmt.Fprint(os.Stdout, `[{"name":"sentinel","source":"registry.example/sentinel:1"}]`)
		} else if containsArguments(arguments, integerCommandName, "build") {
			if err := os.WriteFile(argumentAfter(arguments, "--output"), []byte("image"), 0o600); err != nil {
				os.Exit(95)
			}
		}
		os.Exit(0)
	case "fake-docker":
		arguments := coveragePRHelperArguments()
		if len(arguments) > 0 && arguments[0] == "load" {
			_, _ = fmt.Fprintln(os.Stdout, "Loaded image: local/demo:test")
		} else {
			_, _ = fmt.Fprintf(os.Stdout, `[{"Id":"sha256:%s","Architecture":"amd64","Config":{"User":"65532"}}]`, strings.Repeat("c", 64))
		}
		os.Exit(0)
	case "fake-trivy":
		if err := os.WriteFile(argumentAfter(coveragePRHelperArguments(), "--output"), []byte(`{"Results":[]}`), 0o600); err != nil {
			os.Exit(94)
		}
		os.Exit(0)
	case "wait":
		_, _ = fmt.Fprintln(os.Stdout, "ready")
		select {}
	default:
		os.Exit(96)
	}
}

func coveragePRCommandRequest(mode string) *prCommandRequest {
	return &prCommandRequest{
		Name: os.Args[0],
		Args: []string{"-test.run=^TestCoveragePRCommandHelperProcess$"},
		Env:  []string{coveragePRCommandHelper + "=" + mode},
	}
}

func coveragePRHelperArguments() []string {
	for index, argument := range os.Args {
		if argument == "--" {
			return os.Args[index+1:]
		}
	}
	return nil
}

func TestRunPRCommand_success_preserves_process_boundaries(t *testing.T) {
	// Given: a child process with a sentinel directory, environment, stdin, and tee writers.
	directory := t.TempDir()
	request := coveragePRCommandRequest("success")
	request.Dir = directory
	request.Env = append(request.Env, "PR_SENTINEL=environment")
	request.Stdin = strings.NewReader("standard-input")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	request.Stdout = &stdout
	request.Stderr = &stderr

	// When: the command exits successfully.
	result, err := runPRCommand(t.Context(), request)

	// Then: captured and streamed output preserve all process boundaries.
	require.NoError(t, err)
	require.Equal(t, "stdout|environment|"+filepath.Base(directory)+"|standard-input", string(result.Stdout))
	require.Equal(t, "stderr|sentinel", string(result.Stderr))
	require.Equal(t, result.Stdout, stdout.Bytes())
	require.Equal(t, result.Stderr, stderr.Bytes())
}

func TestRunPRCommand_and_requirePRCommand_distinguish_exit_status(t *testing.T) {
	// Given: a child process that emits diagnostics and exits 23.
	request := coveragePRCommandRequest("exit")

	// When: the permissive runner and strict runner execute the same command.
	result, runErr := runPRCommand(t.Context(), request)
	_, requireErr := requirePRCommand(t.Context(), request)

	// Then: the runner reports the exit as data while the strict runner returns a typed error.
	require.NoError(t, runErr)
	require.Equal(t, 23, result.ExitCode)
	require.Equal(t, "stdout failure", string(result.Stdout))
	require.Equal(t, " stderr failure ", string(result.Stderr))
	require.ErrorIs(t, requireErr, errPRCommandFailed)
	var commandErr *prCommandError
	require.ErrorAs(t, requireErr, &commandErr)
	require.Equal(t, "stdout failure stderr failure", commandErr.Details)
	require.Contains(t, commandErr.Error(), "exited 23")
	require.Equal(t, "sentinel exited 7", (&prCommandError{Name: "sentinel", ExitCode: 7}).Error())
}

func TestRequirePRCommand_accepts_success_and_limits_failure_details(t *testing.T) {
	// Given: one successful command and one failure with more than 4 KiB of output.
	success := coveragePRCommandRequest("success")
	longFailure := coveragePRCommandRequest("long-exit")

	// When: both commands cross the strict command boundary.
	_, successErr := requirePRCommand(t.Context(), success)
	_, failureErr := requirePRCommand(t.Context(), longFailure)

	// Then: success passes through and failure details are bounded.
	require.NoError(t, successErr)
	var commandErr *prCommandError
	require.ErrorAs(t, failureErr, &commandErr)
	require.Len(t, commandErr.Details, 4096)
	require.NotContains(t, commandErr.Details, "stderr-tail")
}

func TestRunPRCommand_cancels_running_process_group(t *testing.T) {
	// Given: a live child that announces readiness before blocking.
	ctx, cancel := context.WithCancel(t.Context())
	readyWriter := &coveragePRReadyWriter{ready: make(chan struct{})}
	request := coveragePRCommandRequest("wait")
	request.Stdout = readyWriter
	request.TermGrace = 250 * time.Millisecond
	done := make(chan coveragePRCommandOutcome, 1)
	go func() {
		result, err := runPRCommand(ctx, request)
		done <- coveragePRCommandOutcome{result: result, err: err}
	}()

	// When: cancellation follows the child's readiness signal.
	select {
	case <-readyWriter.ready:
		cancel()
	case <-time.After(5 * time.Second):
		t.Fatal("child command did not become ready")
	}

	// Then: the process group terminates and the context error is preserved.
	select {
	case outcome := <-done:
		require.ErrorIs(t, outcome.err, context.Canceled)
		require.Equal(t, "ready\n", string(outcome.result.Stdout))
	case <-time.After(5 * time.Second):
		t.Fatal("canceled child command did not exit")
	}
}

func TestRunPRCommand_rejects_invalid_requests_and_start_failures(t *testing.T) {
	// Given: malformed requests, a canceled context, and a missing executable.
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	tests := []struct {
		name, want string
		ctx        context.Context
		request    *prCommandRequest
		wantFailed bool
	}{
		{name: "nil request", ctx: t.Context(), want: "command request is required", wantFailed: true},
		{name: "blank command", ctx: t.Context(), request: &prCommandRequest{Name: " \t"}, want: "command name is required", wantFailed: true},
		{name: "canceled context", ctx: canceled, request: coveragePRCommandRequest("success"), want: "context canceled"},
		{name: "missing executable", ctx: t.Context(), request: &prCommandRequest{Name: filepath.Join(t.TempDir(), "missing")}, want: "start"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// When: the invalid request reaches the process boundary.
			_, err := runPRCommand(test.ctx, test.request)

			// Then: the caller receives the precise validation or startup error.
			require.ErrorContains(t, err, test.want)
			if test.wantFailed {
				require.ErrorIs(t, err, errPRCommandFailed)
			}
		})
	}
}

func TestRunPRCommand_applies_request_timeout(t *testing.T) {
	// Given: a command request whose deadline has no time to start.
	request := coveragePRCommandRequest("success")
	request.Timeout = time.Nanosecond

	// When: the runner applies the request-local timeout.
	_, err := runPRCommand(t.Context(), request)

	// Then: deadline expiry is returned as the causal error.
	require.ErrorIs(t, err, context.DeadlineExceeded)
}
