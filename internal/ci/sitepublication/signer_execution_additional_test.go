package sitepublication

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	errSentinelExecutorExposedSecret = errors.New("executor exposed sentinel-secret")
	errSentinelTransportStopped      = errors.New("transport stopped")
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
		// Given
		command := signerHelperCommand("exit-19", nil)

		// When
		result, err := (ExecRunner{}).Run(context.Background(), command)

		// Then
		require.NoError(t, err)
		assert.Equal(t, 19, result.ExitCode)
		assert.Contains(t, string(result.Stderr), "stderr=exit-19")
	})

	t.Run("canceled before start", func(t *testing.T) {
		// Given
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		stdin := []byte("sentinel-canceled-key")

		// When
		result, err := (ExecRunner{}).Run(ctx, signerHelperCommand("success", stdin))

		// Then
		require.ErrorIs(t, err, context.Canceled)
		assert.Equal(t, make([]byte, len(stdin)), stdin)
		assert.Empty(t, result.Stdout)
	})

	t.Run("missing executor", func(t *testing.T) {
		// Given
		command := ExecutionCommand{Name: filepath.Join(t.TempDir(), "missing-executor")}

		// When
		result, err := (ExecRunner{}).Run(context.Background(), command)

		// Then
		require.ErrorContains(t, err, "start signer command")
		assert.Equal(t, ExecutionResult{}, result)
	})
}

func TestExecuteSigner_succeeds_with_fake_executor_and_sentinel_key(t *testing.T) {
	// Given
	publicationPlan, _ := validPlanAndManifest(t)
	request := signerRequest(t, &publicationPlan, t.TempDir())
	key := writeSignerSentinelKeyPair(t, request)
	expectedKey := append([]byte(nil), key...)
	plan, err := BuildSignerPlan(request)
	require.NoError(t, err)
	runner := &recordingExecutionRunner{run: func(index int, command ExecutionCommand) (ExecutionResult, error) {
		if index < 2 {
			assert.Empty(t, command.Stdin)
			return ExecutionResult{}, nil
		}
		assert.Equal(t, expectedKey, command.Stdin)
		writeSignerFixtureBytes(t, request, filepath.Join(request.OutputAPKPath, "repository-format"), []byte("v1\n"))
		writeSignerFixtureBytes(t, request, filepath.Join(request.OutputAPKPath, "x86_64", "APKINDEX.tar.gz"), []byte("sentinel-index"))
		digest, digestErr := signerRepositoryDigest(filepath.Join(request.WorkspaceDir, request.OutputAPKPath))
		require.NoError(t, digestErr)
		data, marshalErr := json.Marshal(SignerOperationResult{OutputPath: signerOutputPath, OutputDigest: digest})
		require.NoError(t, marshalErr)
		return ExecutionResult{Stdout: data}, nil
	}}

	// When
	result, err := ExecuteSigner(context.Background(), &plan, key, runner)

	// Then
	require.NoError(t, err)
	assert.True(t, result.Signed)
	assert.True(t, result.KeyCleaned)
	assert.Equal(t, plan.SignerDigest, result.SignerDigest)
	assert.Equal(t, request.OutputAPKPath, result.OutputPath)
	assert.True(t, digestPattern.MatchString(string(result.OutputDigest)))
	assert.Len(t, runner.calls, 3)
	assert.Equal(t, make([]byte, len(key)), key)
}

func TestSignerStepExecutor_redacts_and_classifies_runner_errors(t *testing.T) {
	secret := []byte("sentinel-secret")
	tests := []struct {
		name    string
		ctx     func() context.Context
		runErr  error
		wantErr error
	}{
		{name: "generic", ctx: context.Background, runErr: errSentinelExecutorExposedSecret, wantErr: ErrSignerExecution},
		{name: "runner cancellation", ctx: context.Background, runErr: context.Canceled, wantErr: context.Canceled},
		{name: "runner deadline", ctx: context.Background, runErr: context.DeadlineExceeded, wantErr: context.DeadlineExceeded},
		{name: "context cancellation", ctx: canceledSignerContext, runErr: errSentinelTransportStopped, wantErr: context.Canceled},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given
			step := SignerStep{Name: "sentinel-step", Command: ExecutionCommand{Name: "fake", Args: []string{"arg"}, Stdin: []byte("stdin")}}
			executor := signerStepExecutor{
				secret: secret,
				runner: executionRunnerFunc(func(context.Context, ExecutionCommand) (ExecutionResult, error) {
					return ExecutionResult{}, test.runErr
				}),
			}

			// When
			_, err := executor.runStep(test.ctx(), &step)

			// Then
			require.ErrorIs(t, err, test.wantErr)
			assert.NotContains(t, err.Error(), string(secret))
			assert.Empty(t, step.Command.Name)
			assert.Nil(t, step.Command.Args)
			assert.Nil(t, step.Command.Stdin)
		})
	}
}

func TestSignerStepExecutor_redacts_truncates_and_joins_exit_failure(t *testing.T) {
	// Given
	ctx, cancel := context.WithCancel(context.Background())
	secret := []byte("sentinel-secret")
	step := SignerStep{Name: "sign", Command: ExecutionCommand{Name: "fake"}}
	executor := signerStepExecutor{
		secret: secret,
		runner: executionRunnerFunc(func(context.Context, ExecutionCommand) (ExecutionResult, error) {
			cancel()
			stderr := append(append([]byte(nil), secret...), []byte(strings.Repeat("x", 1100))...)
			return ExecutionResult{ExitCode: 7, Stderr: stderr}, nil
		}),
	}

	// When
	_, err := executor.runStep(ctx, &step)

	// Then
	require.ErrorIs(t, err, ErrSignerExecution)
	require.ErrorIs(t, err, context.Canceled)
	assert.NotContains(t, err.Error(), string(secret))
	assert.Contains(t, err.Error(), "…")
}

func TestParseSignerOperationResult_rejects_malformed_unknown_and_trailing_output(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{name: "malformed", data: `{`},
		{name: "unknown field", data: `{"output_path":"/output","output_digest":"` + string(digestOf("a")) + `","unknown":true}`},
		{name: "trailing JSON", data: `{"output_path":"/output","output_digest":"` + string(digestOf("a")) + `"}{}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// When
			result, err := parseSignerOperationResult([]byte(test.data))

			// Then
			require.ErrorIs(t, err, ErrSignerExecution)
			assert.Equal(t, SignerOperationResult{}, result)
		})
	}
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

type executionRunnerFunc func(context.Context, ExecutionCommand) (ExecutionResult, error)

func (run executionRunnerFunc) Run(ctx context.Context, command ExecutionCommand) (ExecutionResult, error) {
	return run(ctx, command)
}

func signerHelperCommand(scenario string, stdin []byte) ExecutionCommand {
	return ExecutionCommand{
		Name:  os.Args[0],
		Args:  []string{"-test.run=^TestSignerExecHelperProcess$", "--", scenario},
		Stdin: stdin,
	}
}

func canceledSignerContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}
