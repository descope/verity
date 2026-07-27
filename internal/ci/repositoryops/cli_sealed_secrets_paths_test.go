package repositoryops

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCLI_verifySealedSecretsImage_usesRunnerTempAndCompletesVerification(t *testing.T) {
	// Given
	tempDirectory := t.TempDir()
	deps := &cliDependencies{
		commands: sealedSecretsCLICommandRunner(t),
		stdout:   &bytes.Buffer{},
		getenv: func(name string) string {
			if name == "RUNNER_TEMP" {
				return tempDirectory
			}
			return ""
		},
	}

	// When
	err := newCLICommand(deps).Run(t.Context(), []string{
		"repository-ops", "verify-sealed-secrets-image", "--image", "ghcr.io/verity/sealed-secrets:v0.30.0",
		"--version", "0.30.0", "--full-version", "0.30.0-r1",
	})

	// Then
	require.NoError(t, err)
	entries, readErr := os.ReadDir(tempDirectory)
	require.NoError(t, readErr)
	require.Empty(t, entries)
}

func sealedSecretsCLICommandRunner(t *testing.T) CommandRunner {
	t.Helper()
	return commandRunnerFunc(func(_ context.Context, command *Command) (CommandResult, error) {
		joined := strings.Join(command.Args, " ")
		switch {
		case strings.Contains(joined, "run --rm --entrypoint /usr/bin/controller") && strings.HasSuffix(joined, " --version"):
			return CommandResult{Stdout: []byte("controller version: v0.30.0\n")}, nil
		case strings.Contains(joined, "run --rm --entrypoint"):
			return CommandResult{}, nil
		case strings.HasPrefix(joined, "create "):
			return CommandResult{Stdout: []byte("sealed-container\n")}, nil
		case joined == "export sealed-container":
			writer := tar.NewWriter(command.Stdout)
			require.NoError(t, writer.WriteHeader(&tar.Header{Name: "etc/ssl/certs/ca-certificates.crt", Mode: 0o644}))
			require.NoError(t, writer.Close())
			return CommandResult{}, nil
		case strings.HasPrefix(joined, "cp sealed-container:"):
			content, err := json.Marshal(map[string]any{
				"spdxVersion": "SPDX-2.3",
				"packages": []map[string]string{{
					"name": "sealed-secrets-0", "versionInfo": "0.30.0-r1", "licenseDeclared": "Apache-2.0",
				}},
			})
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(command.Args[len(command.Args)-1], content, 0o600))
			return CommandResult{}, nil
		case joined == "rm --force sealed-container":
			return CommandResult{}, nil
		default:
			t.Fatalf("unexpected sealed-secrets command: %s", joined)
			return CommandResult{}, nil
		}
	})
}
