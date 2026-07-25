package sitepublication

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

type ExecRunner struct{}

type signerStepExecutor struct {
	runner ExecutionRunner
	secret []byte
}

func (ExecRunner) Run(ctx context.Context, command ExecutionCommand) (ExecutionResult, error) {
	cmd := exec.CommandContext(ctx, command.Name, command.Args...)
	cmd.Env = signerSafeEnvironment(os.Environ())
	if len(command.Stdin) != 0 {
		cmd.Stdin = bytes.NewReader(command.Stdin)
	}
	defer clear(command.Stdin)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	defer func() {
		clear(stdout.Bytes())
		clear(stderr.Bytes())
	}()
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	result := ExecutionResult{
		Stdout: append([]byte(nil), stdout.Bytes()...),
		Stderr: append([]byte(nil), stderr.Bytes()...),
	}
	if err == nil {
		return result, nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return result, ctxErr
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		result.ExitCode = exitError.ExitCode()
		return result, nil
	}
	return ExecutionResult{}, fmt.Errorf("start signer command %s: %w", command.Name, err)
}

func ExecuteSigner(ctx context.Context, plan *SignerPlan, key []byte, runner ExecutionRunner) (result SignerResult, resultErr error) {
	defer func() {
		clear(key)
		if resultErr == nil {
			result.KeyCleaned = true
		}
	}()
	if err := ValidateSignerPlan(plan); err != nil {
		return SignerResult{}, err
	}
	steps, err := buildSignerSteps(plan)
	if err != nil {
		return SignerResult{}, err
	}
	if len(key) == 0 {
		return SignerResult{}, fmt.Errorf("%w: signing key is empty", ErrSignerExecution)
	}
	if runner == nil {
		runner = ExecRunner{}
	}
	executor := &signerStepExecutor{runner: runner, secret: key}
	if err := executor.runPreflight(ctx, steps); err != nil {
		return SignerResult{}, err
	}
	if _, err := validateSignerFilesystem(plan, false); err != nil {
		return SignerResult{}, err
	}
	if err := validateSignerKeyMaterial(plan, key); err != nil {
		return SignerResult{}, fmt.Errorf("%w: key validation failed: %w", ErrSignerExecution, err)
	}
	finalStep := steps[2]
	finalStep.Command.Stdin = append([]byte(nil), key...)
	return executor.executeFinal(ctx, plan, &finalStep)
}

func (executor *signerStepExecutor) executeFinal(
	ctx context.Context,
	plan *SignerPlan,
	step *SignerStep,
) (SignerResult, error) {
	executionResult, err := executor.runStep(ctx, step)
	defer clear(executionResult.Stdout)
	defer clear(executionResult.Stderr)
	if err != nil {
		return SignerResult{}, err
	}
	operationResult, err := parseSignerOperationResult(executionResult.Stdout)
	if err != nil {
		return SignerResult{}, err
	}
	if operationResult.OutputPath != signerOutputPath || !digestPattern.MatchString(string(operationResult.OutputDigest)) {
		return SignerResult{}, fmt.Errorf("%w: invalid signer output result", ErrSignerExecution)
	}
	if _, err := validateSignerFilesystem(plan, false); err != nil {
		return SignerResult{}, err
	}
	outputPath := signerHostPath(plan.Execution.WorkspaceDir, plan.Execution.OutputAPKPath)
	actualDigest, err := signerRepositoryDigest(outputPath)
	if err != nil {
		return SignerResult{}, fmt.Errorf("%w: verify signer output: %w", ErrSignerExecution, err)
	}
	if actualDigest != operationResult.OutputDigest {
		return SignerResult{}, fmt.Errorf("%w: signer output digest mismatch", ErrSignerExecution)
	}
	return SignerResult{
		SignerDigest: plan.SignerDigest, OutputPath: plan.Execution.OutputAPKPath,
		OutputDigest: actualDigest, Signed: true,
	}, nil
}

func (executor *signerStepExecutor) runPreflight(ctx context.Context, steps []SignerStep) error {
	for index := range steps[:2] {
		step := &steps[index]
		step.Command.Stdin = nil
		result, err := executor.runStep(ctx, step)
		clear(result.Stdout)
		clear(result.Stderr)
		if err != nil {
			return err
		}
	}
	return nil
}

func (executor *signerStepExecutor) runStep(ctx context.Context, step *SignerStep) (ExecutionResult, error) {
	defer clearExecutionCommand(&step.Command)
	result, err := executor.runner.Run(ctx, step.Command)
	defer clear(result.Stdout)
	defer clear(result.Stderr)
	if err != nil {
		wrapped := fmt.Errorf("%w: %s: %s", ErrSignerExecution, step.Name, redactSignerText(err.Error(), executor.secret))
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ExecutionResult{}, errors.Join(wrapped, ctxErr)
		}
		if errors.Is(err, context.Canceled) {
			return ExecutionResult{}, errors.Join(wrapped, context.Canceled)
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return ExecutionResult{}, errors.Join(wrapped, context.DeadlineExceeded)
		}
		return ExecutionResult{}, wrapped
	}
	if result.ExitCode != 0 {
		details := truncateSignerText(redactSignerText(string(result.Stderr), executor.secret), 1024)
		wrapped := fmt.Errorf("%w: %s exited %d: %s", ErrSignerExecution, step.Name, result.ExitCode, details)
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ExecutionResult{}, errors.Join(wrapped, ctxErr)
		}
		return ExecutionResult{}, wrapped
	}
	return ExecutionResult{Stdout: append([]byte(nil), result.Stdout...), ExitCode: result.ExitCode}, nil
}

func parseSignerOperationResult(data []byte) (SignerOperationResult, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var result SignerOperationResult
	if err := decoder.Decode(&result); err != nil {
		return SignerOperationResult{}, fmt.Errorf("%w: decode signer output: %w", ErrSignerExecution, err)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return SignerOperationResult{}, fmt.Errorf("%w: trailing signer output", ErrSignerExecution)
	}
	return result, nil
}

func clearExecutionCommand(command *ExecutionCommand) {
	clear(command.Args)
	clear(command.Stdin)
	command.Name = ""
	command.Args = nil
	command.Stdin = nil
}

func signerSafeEnvironment(environment []string) []string {
	allowed := make(map[string]string)
	for _, entry := range environment {
		name, _, found := strings.Cut(entry, "=")
		if !found {
			continue
		}
		switch name {
		case "GH_TOKEN", "GITHUB_TOKEN", "SSL_CERT_FILE", "SSL_CERT_DIR":
			if _, exists := allowed[name]; !exists {
				allowed[name] = entry
			}
		}
	}
	filtered := []string{"PATH=/usr/bin:/bin"}
	for _, name := range []string{"GH_TOKEN", "GITHUB_TOKEN", "SSL_CERT_FILE", "SSL_CERT_DIR"} {
		if entry, exists := allowed[name]; exists {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

func redactSignerText(value string, secret []byte) string {
	if len(secret) == 0 {
		return value
	}
	return strings.ReplaceAll(value, string(secret), "[REDACTED]")
}

func truncateSignerText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "…"
}
