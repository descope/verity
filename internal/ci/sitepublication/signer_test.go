package sitepublication

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordingExecutionRunner struct {
	calls []ExecutionCommand
	run   func(int, ExecutionCommand) (ExecutionResult, error)
}

var (
	errPullUnavailable   = errors.New("pull unavailable")
	errRuntimeDiagnostic = errors.New("runtime diagnostic")
)

func (runner *recordingExecutionRunner) Run(_ context.Context, command ExecutionCommand) (ExecutionResult, error) {
	runner.calls = append(runner.calls, command)
	if runner.run == nil {
		return ExecutionResult{}, nil
	}
	return runner.run(len(runner.calls)-1, command)
}

func TestBuildSignerPlan_constructs_ordered_isolated_minimal_commands(t *testing.T) {
	// Given a pinned plan and dedicated signing workspace/key directory.
	plan, _ := validPlanAndManifest(t)
	root := t.TempDir()
	request := signerRequest(t, &plan, root)

	// When a signer execution plan is constructed and validated.
	signerPlan, err := BuildSignerPlan(request)
	require.NoError(t, err)
	err = ValidateSignerPlan(&signerPlan)

	// Then pull and attestation precede the only key-bearing signer command.
	require.NoError(t, err)
	require.Len(t, signerPlan.Steps, 3)
	assert.Equal(t, []string{"pull", "attest", "sign"}, []string{signerPlan.Steps[0].Name, signerPlan.Steps[1].Name, signerPlan.Steps[2].Name})
	assert.False(t, signerPlan.Steps[0].KeyAccess)
	assert.False(t, signerPlan.Steps[1].KeyAccess)
	assert.True(t, signerPlan.Steps[2].KeyAccess)
	signArgs := signerPlan.Steps[2].Command.Args
	assert.Empty(t, signerPlan.Steps[2].Command.Stdin)
	for _, required := range []string{
		"--network=none", "--read-only", "--user=65532:65532", "--cap-drop=ALL",
		"--security-opt=no-new-privileges", "--pids-limit=256",
	} {
		assert.Contains(t, signArgs, required)
	}
	assert.Contains(t, signArgs, "--entrypoint=/usr/bin/apk-repository-signer")
	assert.Contains(t, signArgs, "--tmpfs="+signerTmpfsArgument)
	assert.Equal(t, 6, countArgumentPrefix(signArgs, "--mount="))
	assert.NotContains(t, strings.Join(signArgs, " "), "APK_REPOSITORY_PRIVATE_KEY")
	assert.NotContains(t, strings.Join(signArgs, " "), request.KeyDirectory)
	assert.NotContains(t, signArgs, "--signing-key")
	assert.Equal(t, request.Plan.SignerReference, signerPlan.ImageReference)
	assert.Contains(t, signerPlan.Steps[1].Command.Args, "--source-digest")
	assert.Contains(t, signerPlan.Steps[1].Command.Args, testSignerSourceSHA)
}

func TestExecuteSigner_passes_key_on_stdin_only_after_attestation(t *testing.T) {
	// Given a valid plan and a fake key that must never appear in arguments or errors.
	plan, _ := validPlanAndManifest(t)
	root := t.TempDir()
	request := signerRequest(t, &plan, root)
	secret := writeSignerSentinelKeyPair(t, request)
	signerPlan, err := BuildSignerPlan(request)
	require.NoError(t, err)
	runner := &recordingExecutionRunner{}
	runner.run = func(index int, command ExecutionCommand) (ExecutionResult, error) {
		if index < 2 {
			assert.NoDirExists(t, signerPlan.Cleanup.KeyDirectory)
			assert.Empty(t, command.Stdin)
			return ExecutionResult{}, nil
		}
		assert.Equal(t, secret, command.Stdin)
		assert.NoDirExists(t, signerPlan.Cleanup.KeyDirectory)
		assert.NotContains(t, command.Name+strings.Join(command.Args, " "), string(secret))
		return ExecutionResult{ExitCode: 1, Stderr: append([]byte("signer echoed "), secret...)}, nil
	}

	// When signing fails after pull and attestation.
	_, err = ExecuteSigner(context.Background(), &signerPlan, secret, runner)

	// Then returned diagnostics redact the key and no host key material is created.
	require.Error(t, err)
	assert.NotContains(t, err.Error(), string(secret))
	assert.NoDirExists(t, signerPlan.Cleanup.KeyDirectory)
	assert.Len(t, runner.calls, 3)
}

func TestExecuteSigner_does_not_materialize_host_key_when_preflight_fails(t *testing.T) {
	// Given a pull failure before signer attestation.
	plan, _ := validPlanAndManifest(t)
	root := t.TempDir()
	signerPlan, err := BuildSignerPlan(signerRequest(t, &plan, root))
	require.NoError(t, err)
	runner := &recordingExecutionRunner{run: func(_ int, _ ExecutionCommand) (ExecutionResult, error) {
		return ExecutionResult{}, errPullUnavailable
	}}

	// When signer execution begins.
	_, err = ExecuteSigner(context.Background(), &signerPlan, []byte("secret"), runner)

	// Then no host key is written and later steps do not run.
	require.Error(t, err)
	assert.NoDirExists(t, signerPlan.Cleanup.KeyDirectory)
	assert.Len(t, runner.calls, 1)
}

func TestExecuteSigner_redacts_parent_key_from_preflight_errors(t *testing.T) {
	// Given a parent-process key and a hostile runtime diagnostic containing that value.
	plan, _ := validPlanAndManifest(t)
	signerPlan, err := BuildSignerPlan(signerRequest(t, &plan, t.TempDir()))
	require.NoError(t, err)
	secret := []byte("PARENT-KEY-MUST-NOT-LEAK")
	runner := &recordingExecutionRunner{run: func(_ int, _ ExecutionCommand) (ExecutionResult, error) {
		return ExecutionResult{}, fmt.Errorf("%w: %s", errRuntimeDiagnostic, secret)
	}}

	// When preflight fails before stdin key handling.
	_, err = ExecuteSigner(context.Background(), &signerPlan, secret, runner)

	// Then even parent-process diagnostics are redacted.
	require.Error(t, err)
	assert.NotContains(t, err.Error(), string(secret))
}

func TestValidateSignerPlan_rejects_wrong_image_and_isolation_regression(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*SignerPlan)
	}{
		{name: "wrong image", mutate: func(plan *SignerPlan) { plan.ImageReference = "ghcr.io/attacker/signer@" + string(digestOf("a")) }},
		{name: "network enabled", mutate: func(plan *SignerPlan) {
			replaceArgument(&plan.Steps[2].Command.Args, "--network=none", "--network=bridge")
		}},
		{name: "key before attestation", mutate: func(plan *SignerPlan) { plan.Steps[0].KeyAccess = true }},
		{name: "missing signer source", mutate: func(plan *SignerPlan) { plan.SignerSourceSHA = "" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given one hostile signer-plan mutation.
			publicationPlan, _ := validPlanAndManifest(t)
			plan, err := BuildSignerPlan(signerRequest(t, &publicationPlan, t.TempDir()))
			require.NoError(t, err)
			test.mutate(&plan)

			// When the plan is checked before secret handling.
			err = ValidateSignerPlan(&plan)

			// Then isolation validation fails closed.
			require.ErrorIs(t, err, ErrInvalidSignerPlan)
		})
	}
}

func signerRequest(t *testing.T, plan *PublicationPlan, root string) *SignerRequest {
	t.Helper()
	request := &SignerRequest{
		Plan: *plan, Runtime: "docker", Repository: "verity-org/verity",
		WorkspaceDir: filepath.Join(root, "workspace"), KeyDirectory: filepath.Join(root, "key"),
		ManifestPath: "publication.json", PackagesPath: "packages", BaseAPKPath: "previous/apk",
		DeltaManifestPath: "apk-delta.json", OutputAPKPath: "signed-apk", PublicKeyPath: "verity.rsa.pub",
	}
	require.NoError(t, os.MkdirAll(request.WorkspaceDir, 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(request.WorkspaceDir, request.OutputAPKPath), 0o755))
	writeBoundSignerFixtures(t, request)
	writeSignerFixtureFile(t, request, request.PublicKeyPath)
	return request
}

func writeSignerFixtureFile(t *testing.T, request *SignerRequest, relative string) {
	t.Helper()
	path := filepath.Join(request.WorkspaceDir, filepath.FromSlash(relative))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("signer-fixture"), 0o644))
}

func countArgumentPrefix(arguments []string, prefix string) int {
	count := 0
	for _, argument := range arguments {
		if strings.HasPrefix(argument, prefix) {
			count++
		}
	}
	return count
}

func replaceArgument(arguments *[]string, old, replacement string) {
	for index := range *arguments {
		if (*arguments)[index] == old {
			(*arguments)[index] = replacement
			return
		}
	}
}
