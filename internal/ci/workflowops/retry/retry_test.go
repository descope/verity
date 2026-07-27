package retry

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessRunner_times_out_a_hung_command(t *testing.T) {
	// Given: a command runner that blocks until its attempt context is cancelled.
	var stderr bytes.Buffer
	process := Process{
		Runner: blockingRunner{},
		Engine: Engine{Sleeper: noWaitSleeper{}, Random: zeroRandom{}},
		Stderr: &stderr,
	}
	operation := Operation{
		Label: "hung command",
		Command: Command{
			Name:    "hung",
			Timeout: 10 * time.Millisecond,
		},
	}

	// When: the command exceeds its per-attempt timeout.
	err := process.Run(t.Context(), &operation, Policy{MaxAttempts: 1})

	// Then: the timeout is returned instead of leaving the workflow hung.
	require.Error(t, err)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Contains(t, stderr.String(), "hung command")
}

func TestProcessRunner_retries_with_linear_jitter_and_preserves_output(t *testing.T) {
	// Given: a command that prints misleading output, fails once, then succeeds.
	runner := &sequenceRunner{
		results: []Result{
			{Stdout: []byte("success-looking output\n"), ExitCode: 7},
			{Stdout: []byte("sha256:abc"), ExitCode: 0},
		},
		errors: []error{fmt.Errorf("first attempt: %w", &CommandError{ExitCode: 7}), nil},
	}
	sleeper := &recordingSleeper{}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	process := Process{
		Runner: runner,
		Engine: Engine{Sleeper: sleeper, Random: fixedRandom{value: int(2 * time.Second)}},
		Stdout: &stdout,
		Stderr: &stderr,
	}

	// When: retry policy permits a second attempt.
	operation := Operation{Label: "flaky", Command: Command{Name: "flaky"}}
	err := process.Run(t.Context(), &operation, Policy{
		MaxAttempts: 2,
		BaseDelay:   10 * time.Second,
		Jitter:      5 * time.Second,
	})

	// Then: both outputs are preserved, the failure is not mistaken for success, and timing is deterministic.
	require.NoError(t, err)
	assert.Equal(t, 2, runner.calls)
	assert.Equal(t, []time.Duration{12 * time.Second}, sleeper.delays)
	assert.Equal(t, "success-looking output\nsha256:abc", stdout.String())
	assert.Contains(t, stderr.String(), "flaky failed (exit 7); retrying in 12s")
}

func TestExecRunner_returns_typed_exit_without_exposing_stdin(t *testing.T) {
	// Given: a real process that consumes a credential from stdin and exits unsuccessfully.
	secret := "not-for-logs"

	// When: the process exits with a non-zero status.
	command := Command{
		Name:  "bash",
		Args:  []string{"-c", "read -r secret; printf diagnostic >&2; exit 9"},
		Stdin: []byte(secret + "\n"),
	}
	result, err := (ExecRunner{}).Run(t.Context(), &command)

	// Then: callers can branch on the exit code without the credential entering output or errors.
	require.Error(t, err)
	var commandErr *CommandError
	require.ErrorAs(t, err, &commandErr)
	assert.Equal(t, 9, commandErr.ExitCode)
	assert.Equal(t, "diagnostic", string(result.Stderr))
	assert.NotContains(t, err.Error(), secret)
	assert.NotContains(t, string(result.Stdout)+string(result.Stderr), secret)
}

func TestDockerLoginOperation_keeps_password_out_of_arguments(t *testing.T) {
	// Given: registry credentials supplied at the trusted CLI boundary.
	password := "registry-secret"

	// When: a typed docker-login operation is constructed.
	operation, policy, err := NewDockerLoginOperation(&DockerLoginOptions{
		Registry:  "ghcr.io",
		Username:  "verity",
		Password:  password,
		Attempts:  4,
		Timeout:   45 * time.Second,
		BaseDelay: 10 * time.Second,
		Jitter:    10 * time.Second,
	})

	// Then: the password is stdin-only and retry/timeout defaults remain explicit typed data.
	require.NoError(t, err)
	assert.Equal(t, []string{"login", "ghcr.io", "--username", "verity", "--password-stdin"}, operation.Command.Args)
	assert.Equal(t, password, string(operation.Command.Stdin))
	assert.NotContains(t, operation.Label+operation.Command.Name+fmt.Sprint(operation.Command.Args), password)
	assert.Equal(t, 45*time.Second, operation.Command.Timeout)
	assert.Equal(t, 4, policy.MaxAttempts)
}

func TestEngine_rejects_jitter_without_random_source(t *testing.T) {
	// Given: a retry policy requiring jitter but no injected random source.
	engine := Engine{Sleeper: noWaitSleeper{}}

	// When: retry execution starts.
	err := engine.Do(t.Context(), Policy{MaxAttempts: 2, Jitter: time.Second}, func(context.Context, int) error {
		return &CommandError{ExitCode: 1}
	})

	// Then: configuration fails instead of silently changing retry timing.
	require.ErrorIs(t, err, ErrInvalidPolicy)
}

func TestEngine_bounds_each_attempt_and_does_not_sleep_after_cancellation(t *testing.T) {
	// Given: an attempt that blocks until its per-attempt deadline and a retry delay.
	sleeper := &recordingSleeper{}
	engine := Engine{Sleeper: sleeper, Random: zeroRandom{}}
	started := time.Now()

	// When: the first attempt exceeds its configured timeout.
	err := engine.Do(t.Context(), Policy{
		MaxAttempts: 2, BaseDelay: time.Second, AttemptTimeout: 20 * time.Millisecond,
	}, func(ctx context.Context, _ int) error {
		<-ctx.Done()
		return ctx.Err()
	})

	// Then: cancellation returns promptly and no retry sleep starts.
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, time.Since(started), 200*time.Millisecond)
	assert.Empty(t, sleeper.delays)
}

type blockingRunner struct{}

func (blockingRunner) Run(ctx context.Context, _ *Command) (Result, error) {
	<-ctx.Done()
	return Result{}, ctx.Err()
}

type noWaitSleeper struct{}

func (noWaitSleeper) Wait(context.Context, time.Duration) error {
	return nil
}

type zeroRandom struct{}

func (zeroRandom) Intn(int) (int, error) {
	return 0, nil
}

type fixedRandom struct {
	value int
}

func (random fixedRandom) Intn(int) (int, error) {
	return random.value, nil
}

type recordingSleeper struct {
	delays []time.Duration
}

func (sleeper *recordingSleeper) Wait(_ context.Context, delay time.Duration) error {
	sleeper.delays = append(sleeper.delays, delay)
	return nil
}

type sequenceRunner struct {
	results []Result
	errors  []error
	calls   int
}

func (runner *sequenceRunner) Run(context.Context, *Command) (Result, error) {
	index := runner.calls
	runner.calls++
	return runner.results[index], runner.errors[index]
}
