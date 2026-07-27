package apkrepository

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecCommandRunner_captures_output_and_exit_status(t *testing.T) {
	// Given the real process adapter.
	runner := execCommandRunner{}

	// When successful and failing commands execute.
	success, successErr := runner.Run(context.Background(), &command{
		name: "sh", args: []string{"-c", "printf stdout; printf stderr >&2"},
	})
	failure, failureErr := runner.Run(context.Background(), &command{
		name: "sh", args: []string{"-c", "printf failed >&2; exit 7"},
	})

	// Then output streams and process exit codes remain typed and inspectable.
	require.NoError(t, successErr)
	assert.Equal(t, "stdout", string(success.stdout))
	assert.Equal(t, "stderr", string(success.stderr))
	assert.Zero(t, success.exitCode)
	require.NoError(t, failureErr)
	assert.Equal(t, 7, failure.exitCode)
	assert.Equal(t, "failed", string(failure.stderr))
}

func TestRunRequired_rejects_nonzero_exit_and_start_failure(t *testing.T) {
	// Given a real runner and one missing executable.
	runner := execCommandRunner{}

	// When required commands fail by exit status and startup error.
	_, exitErr := runRequired(context.Background(), runner, &command{
		name: "sh", args: []string{"-c", "printf details >&2; exit 9"},
	})
	_, startErr := runRequired(context.Background(), runner, &command{name: "verity-command-that-does-not-exist"})

	// Then both failures are surfaced with their distinct evidence.
	require.ErrorIs(t, exitErr, errCommandFailed)
	assert.ErrorContains(t, exitErr, "details")
	require.Error(t, startErr)
	assert.ErrorContains(t, startErr, "start verity-command-that-does-not-exist")
}
