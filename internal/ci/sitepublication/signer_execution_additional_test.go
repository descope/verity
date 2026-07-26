package sitepublication

import (
	"context"
	"encoding/json"
	"errors"
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

type executionRunnerFunc func(context.Context, ExecutionCommand) (ExecutionResult, error)

func (run executionRunnerFunc) Run(ctx context.Context, command ExecutionCommand) (ExecutionResult, error) {
	return run(ctx, command)
}

func canceledSignerContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}
