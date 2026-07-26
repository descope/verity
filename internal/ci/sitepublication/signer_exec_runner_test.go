package sitepublication

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSignerExecRunner_runs_fake_executor_with_stdin_and_safe_environment(t *testing.T) {
	// Given
	t.Setenv("GH_TOKEN", "sentinel-gh-token")
	t.Setenv("UNSAFE_SENTINEL", "must-not-pass")
	stdin := []byte("sentinel-stdin-key")
	command := signerHelperCommand("success", stdin)

	// When
	result, err := (ExecRunner{}).Run(context.Background(), command)

	// Then
	require.NoError(t, err)
	assert.Zero(t, result.ExitCode)
	output := string(result.Stdout)
	assert.Contains(t, output, "stdin=sentinel-stdin-key")
	assert.Contains(t, output, "gh=sentinel-gh-token")
	assert.Contains(t, output, "unsafe=")
	assert.NotContains(t, output, "must-not-pass")
	assert.Contains(t, output, "path=/usr/bin:/bin")
	assert.Contains(t, string(result.Stderr), "stderr=sentinel")
	assert.Equal(t, make([]byte, len(stdin)), stdin)
}

func TestSignerExecRunner_returns_exit_cancellation_and_start_failures(t *testing.T) {
	t.Run("nonzero exit", func(t *testing.T) {
		result, err := (ExecRunner{}).Run(context.Background(), signerHelperCommand("exit-19", nil))
		require.NoError(t, err)
		assert.Equal(t, 19, result.ExitCode)
		assert.Contains(t, string(result.Stderr), "stderr=exit-19")
	})

	t.Run("canceled before start", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		stdin := []byte("sentinel-canceled-key")
		result, err := (ExecRunner{}).Run(ctx, signerHelperCommand("success", stdin))
		require.ErrorIs(t, err, context.Canceled)
		assert.Equal(t, make([]byte, len(stdin)), stdin)
		assert.Empty(t, result.Stdout)
	})

	t.Run("missing executor", func(t *testing.T) {
		command := ExecutionCommand{Name: filepath.Join(t.TempDir(), "missing-executor")}
		result, err := (ExecRunner{}).Run(context.Background(), command)
		require.ErrorContains(t, err, "start signer command")
		assert.Equal(t, ExecutionResult{}, result)
	})
}

func TestSignerExecHelperProcess(t *testing.T) {
	separator := -1
	for index, argument := range os.Args {
		if argument == "--" {
			separator = index
			break
		}
	}
	if separator == -1 || separator+1 >= len(os.Args) {
		return
	}
	scenario := os.Args[separator+1]
	switch scenario {
	case "success":
		stdin, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		fmt.Fprintf(os.Stdout, "stdin=%s gh=%s unsafe=%s path=%s\n", stdin, os.Getenv("GH_TOKEN"), os.Getenv("UNSAFE_SENTINEL"), os.Getenv("PATH"))
		fmt.Fprintln(os.Stderr, "stderr=sentinel")
	case "exit-19":
		fmt.Fprintln(os.Stderr, "stderr=exit-19")
		os.Exit(19)
	default:
		fmt.Fprintln(os.Stderr, "unknown helper scenario: "+scenario)
		os.Exit(3)
	}
}

func signerHelperCommand(scenario string, stdin []byte) ExecutionCommand {
	return ExecutionCommand{
		Name:  os.Args[0],
		Args:  []string{"-test.run=^TestSignerExecHelperProcess$", "--", scenario},
		Stdin: stdin,
	}
}
