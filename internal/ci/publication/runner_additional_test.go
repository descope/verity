package publication

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecRunner_runs_fake_process_in_configured_directory(t *testing.T) {
	// Given
	root := t.TempDir()
	runner := ExecRunner{Dir: root}
	command := publicationHelperCommand("success")

	// When
	result, err := runner.Run(context.Background(), command)

	// Then
	require.NoError(t, err)
	assert.Zero(t, result.ExitCode)
	assert.Contains(t, string(result.Stdout), "stdout="+root)
	assert.Contains(t, string(result.Stderr), "stderr=sentinel")
}

func TestExecRunner_returns_fake_process_exit_status_without_transport_error(t *testing.T) {
	// Given
	runner := ExecRunner{}
	command := publicationHelperCommand("exit-23")

	// When
	result, err := runner.Run(context.Background(), command)

	// Then
	require.NoError(t, err)
	assert.Equal(t, 23, result.ExitCode)
	assert.Contains(t, string(result.Stderr), "stderr=exit-23")
}

func TestExecRunner_returns_context_cancellation_before_start(t *testing.T) {
	// Given
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// When
	result, err := (ExecRunner{}).Run(ctx, publicationHelperCommand("success"))

	// Then
	require.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, result.Stdout)
	assert.Empty(t, result.Stderr)
}

func TestExecRunner_wraps_missing_fake_process_start_error(t *testing.T) {
	// Given
	command := Command{Name: filepath.Join(t.TempDir(), "missing-executor")}

	// When
	result, err := (ExecRunner{}).Run(context.Background(), command)

	// Then
	require.ErrorContains(t, err, "start "+command.Name)
	assert.Empty(t, result.Stdout)
	assert.Empty(t, result.Stderr)
}

func TestPublicationExecHelperProcess(t *testing.T) {
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
		workingDirectory, err := os.Getwd()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		fmt.Fprintln(os.Stdout, "stdout="+workingDirectory)
		fmt.Fprintln(os.Stderr, "stderr=sentinel")
	case "exit-23":
		fmt.Fprintln(os.Stderr, "stderr=exit-23")
		os.Exit(23)
	default:
		fmt.Fprintln(os.Stderr, "unknown helper scenario: "+scenario)
		os.Exit(3)
	}
}

func publicationHelperCommand(scenario string) Command {
	return Command{
		Name: os.Args[0],
		Args: []string{"-test.run=^TestPublicationExecHelperProcess$", "--", strings.TrimSpace(scenario)},
	}
}
