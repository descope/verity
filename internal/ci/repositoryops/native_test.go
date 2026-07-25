package repositoryops_test

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ops "github.com/verity-org/verity/internal/ci/repositoryops"
)

func TestNativeService_TestPackage_buildsExactRcloneCommand(t *testing.T) {
	// Given
	root := t.TempDir()
	request, err := ops.NewNativePackageRequest(ops.NativePackageInput{Kind: "rclone", Architecture: "x86_64", RepositoryRoot: root})
	require.NoError(t, err)
	runner := &fakeCommandRunner{responses: []ops.CommandResult{{}}}

	// When
	err = (ops.NativeService{Commands: runner}).TestPackage(context.Background(), &request)

	// Then
	require.NoError(t, err)
	require.Len(t, runner.calls, 1)
	assert.Equal(t, "melange", runner.calls[0].Name)
	assert.Equal(t, []string{
		"test", "--arch", "x86_64",
		"--repository-append", filepath.Join(root, "packages", "repo"),
		"--repository-append", "https://packages.wolfi.dev/os",
		"--keyring-append", filepath.Join(root, "melange-work", "melange.rsa.pub"),
		"--keyring-append", "https://packages.wolfi.dev/os/wolfi-signing.rsa.pub",
		"--runner", "docker", "--pipeline-dirs", "melange-work/pipelines",
		"melange-work/specs/rclone.yaml/build.yaml", "rclone",
	}, runner.calls[0].Args)
	_, hasDeadline := runner.contexts[0].Deadline()
	assert.True(t, hasDeadline)
}

func TestNewNativePackageRequest_rejectsUnsupportedArchitecture(t *testing.T) {
	// When
	_, err := ops.NewNativePackageRequest(ops.NativePackageInput{Kind: "step-ca", Architecture: "$(id)", RepositoryRoot: t.TempDir()})

	// Then
	require.ErrorIs(t, err, ops.ErrUnsupportedArchitecture)
}

func TestNativeService_TestPackage_preservesEveryPackageRecipe(t *testing.T) {
	tests := []struct {
		kind        string
		buildFile   string
		packageName string
	}{
		{kind: "rclone", buildFile: "melange-work/specs/rclone.yaml/build.yaml", packageName: "rclone"},
		{kind: "sealed-secrets", buildFile: "melange-work/specs/sealed-secrets-0.yaml/build.yaml", packageName: "sealed-secrets-0"},
		{kind: "step-ca", buildFile: "melange-work/specs/step-ca.yaml/build.yaml", packageName: "step-ca"},
	}
	for _, test := range tests {
		t.Run(test.kind, func(t *testing.T) {
			// Given
			runner := &fakeCommandRunner{responses: []ops.CommandResult{{}}}
			request, err := ops.NewNativePackageRequest(ops.NativePackageInput{Kind: test.kind, Architecture: "aarch64", RepositoryRoot: t.TempDir()})
			require.NoError(t, err)

			// When
			err = (ops.NativeService{Commands: runner}).TestPackage(context.Background(), &request)

			// Then
			require.NoError(t, err)
			assert.Equal(t, test.buildFile, runner.calls[0].Args[len(runner.calls[0].Args)-2])
			assert.Equal(t, test.packageName, runner.calls[0].Args[len(runner.calls[0].Args)-1])
		})
	}
}

func TestNativeService_VerifySealedSecretsImage_checksRuntimeAndSBOMThenCleansUp(t *testing.T) {
	// Given
	tempDir := t.TempDir()
	request, err := ops.NewSealedSecretsImageRequest(ops.SealedSecretsImageInput{
		Image:       "ghcr.io/verity/sealed-secrets:v0.30.0",
		Version:     "0.30.0",
		FullVersion: "0.30.0-r1",
		TempDir:     tempDir,
	})
	require.NoError(t, err)
	runner := sealedSecretsRunner(t, "0.30.0-r1")

	// When
	err = (ops.NativeService{Commands: runner}).VerifySealedSecretsImage(context.Background(), request)

	// Then
	require.NoError(t, err)
	commands := make([]string, 0, len(runner.calls))
	for _, call := range runner.calls {
		commands = append(commands, call.Name+" "+strings.Join(call.Args, " "))
	}
	assert.Contains(t, commands, "docker rm --force sealed-container")
	entries, err := os.ReadDir(tempDir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestNativeService_VerifySealedSecretsImage_surfacesCleanupFailure(t *testing.T) {
	// Given
	request, err := ops.NewSealedSecretsImageRequest(ops.SealedSecretsImageInput{
		Image:       "ghcr.io/verity/sealed-secrets:v0.30.0",
		Version:     "0.30.0",
		FullVersion: "0.30.0-r1",
		TempDir:     t.TempDir(),
	})
	require.NoError(t, err)
	runner := sealedSecretsRunner(t, "0.30.0-r1")
	baseRun := runner.run
	runner.run = func(ctx context.Context, command ops.Command, callIndex int) (ops.CommandResult, error) {
		if strings.Join(command.Args, " ") == "rm --force sealed-container" {
			return ops.CommandResult{ExitCode: 1, Stderr: []byte("cleanup denied")}, nil
		}
		return baseRun(ctx, command, callIndex)
	}

	// When
	err = (ops.NativeService{Commands: runner}).VerifySealedSecretsImage(context.Background(), request)

	// Then
	require.ErrorIs(t, err, ops.ErrCommandFailed)
}

func sealedSecretsRunner(t *testing.T, fullVersion string) *fakeCommandRunner {
	t.Helper()
	return &fakeCommandRunner{run: func(_ context.Context, command ops.Command, _ int) (ops.CommandResult, error) {
		joined := strings.Join(command.Args, " ")
		switch {
		case strings.Contains(joined, "run --rm --entrypoint /usr/bin/controller") && strings.HasSuffix(joined, " --version"):
			return ops.CommandResult{Stdout: []byte("controller version: v0.30.0\n")}, nil
		case strings.Contains(joined, "run --rm --entrypoint"):
			return ops.CommandResult{}, nil
		case strings.HasPrefix(joined, "create "):
			return ops.CommandResult{Stdout: []byte("sealed-container\n")}, nil
		case joined == "export sealed-container":
			writeCertificateTar(t, &command)
			return ops.CommandResult{}, nil
		case strings.HasPrefix(joined, "cp sealed-container:"):
			writeSBOM(t, command.Args[len(command.Args)-1], fullVersion)
			return ops.CommandResult{}, nil
		case joined == "rm --force sealed-container":
			return ops.CommandResult{}, nil
		default:
			return ops.CommandResult{}, assert.AnError
		}
	}}
}

func writeCertificateTar(t *testing.T, command *ops.Command) {
	t.Helper()
	require.NotNil(t, command.Stdout)
	writer := tar.NewWriter(command.Stdout)
	require.NoError(t, writer.WriteHeader(&tar.Header{Name: "etc/ssl/certs/ca-certificates.crt", Mode: 0o644, Size: 0}))
	require.NoError(t, writer.Close())
}

func writeSBOM(t *testing.T, path, fullVersion string) {
	t.Helper()
	data, err := json.Marshal(map[string]any{
		"spdxVersion": "SPDX-2.3",
		"packages": []map[string]string{{
			"name": "sealed-secrets-0", "versionInfo": fullVersion, "licenseDeclared": "Apache-2.0",
		}},
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, bytes.TrimSpace(data), 0o600))
}
