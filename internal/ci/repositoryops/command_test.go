package repositoryops_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ops "github.com/verity-org/verity/internal/ci/repositoryops"
)

func TestExecCommandRunner_passesMaliciousArgumentLiterally(t *testing.T) {
	// Given
	marker := filepath.Join(t.TempDir(), "pwned")
	argument := "$(touch " + marker + ")"

	// When
	result, err := (ops.ExecCommandRunner{}).Run(context.Background(), &ops.Command{Name: "printf", Args: []string{"%s", argument}})

	// Then
	require.NoError(t, err)
	assert.Equal(t, argument, string(result.Stdout))
	assert.NoFileExists(t, marker)
}

func TestExecCommandRunner_terminatesHungCommandOnCancellation(t *testing.T) {
	// Given
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()

	// When
	_, err := (ops.ExecCommandRunner{}).Run(ctx, &ops.Command{Name: "sh", Args: []string{"-c", "trap '' TERM; while :; do sleep 10; done"}})

	// Then
	require.Error(t, err)
	assert.True(t, errors.Is(err, context.DeadlineExceeded), err)
	assert.Less(t, time.Since(started), 3*time.Second)
}

func TestExecCommandRunner_handlesRepeatedInterruptions(t *testing.T) {
	for range 2 {
		// Given
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		// When
		_, err := (ops.ExecCommandRunner{}).Run(ctx, &ops.Command{Name: "sh", Args: []string{"-c", "exit 0"}})

		// Then
		require.Error(t, err)
		assert.True(t, errors.Is(err, context.Canceled), err)
	}
}

func TestExecCommandRunner_runsNextCommandAfterCancellation(t *testing.T) {
	// Given
	runner := ops.ExecCommandRunner{}
	canceledContext, cancel := context.WithCancel(t.Context())
	cancel()
	_, canceledErr := runner.Run(canceledContext, &ops.Command{Name: "printf", Args: []string{"ignored"}})
	require.ErrorIs(t, canceledErr, context.Canceled)

	// When
	result, err := runner.Run(t.Context(), &ops.Command{Name: "printf", Args: []string{"resumed"}})

	// Then
	require.NoError(t, err)
	assert.Equal(t, "resumed", string(result.Stdout))
}
