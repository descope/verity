package apkrepository

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidate_verify_crypto_accepts_trusted_clients_and_rejects_wrong_key(t *testing.T) {
	// Given a complete two-architecture signed repository and deterministic apk process results.
	repository := t.TempDir()
	writeTestFile(t, filepath.Join(repository, "verity.rsa.pub"), "public key")
	for _, architecture := range []string{"x86_64", "aarch64"} {
		writeTestFile(t, filepath.Join(repository, architecture, "demo.apk"), "package")
		writeTestIndex(t, filepath.Join(repository, architecture, "APKINDEX.tar.gz"), ".SIGN.RSA256.verity.rsa.pub", "APKINDEX")
	}
	runner := cryptoValidationRunner()

	// When publication-grade crypto validation runs.
	err := Validate(context.Background(), &ValidateOptions{
		RepositoryDir: repository, VerifyCrypto: true, runner: runner,
	})

	// Then trusted package/index checks pass and both wrong-key checks are exercised.
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(runner.calls), 10)
	assert.True(t, slices.ContainsFunc(runner.calls, func(call command) bool {
		return call.name == "apk" && slices.Contains(call.args, "verify") && slices.ContainsFunc(call.args, func(arg string) bool {
			return strings.Contains(arg, "wrong-keys")
		})
	}))
}

func TestValidate_verify_crypto_requires_both_primary_architectures(t *testing.T) {
	// Given a signed x86_64-only repository.
	repository := t.TempDir()
	writeTestFile(t, filepath.Join(repository, "verity.rsa.pub"), "public key")
	writeTestFile(t, filepath.Join(repository, "x86_64", "demo.apk"), "package")
	writeTestIndex(t, filepath.Join(repository, "x86_64", "APKINDEX.tar.gz"), ".SIGN.RSA256.verity.rsa.pub")

	// When crypto validation runs.
	err := Validate(context.Background(), &ValidateOptions{
		RepositoryDir: repository, VerifyCrypto: true, runner: cryptoValidationRunner(),
	})

	// Then the missing architecture fails before external verification begins.
	require.ErrorIs(t, err, errRequiredArchitectureMissing)
	assert.ErrorContains(t, err, "aarch64")
}

func cryptoValidationRunner() *fakeCommandRunner {
	return &fakeCommandRunner{run: func(request command) (commandResult, error) {
		if request.name != "apk" {
			return commandResult{}, assert.AnError
		}
		joined := strings.Join(request.args, " ")
		switch {
		case strings.Contains(joined, "wrong-keys") && slices.Contains(request.args, "verify"):
			return commandResult{stderr: []byte("UNTRUSTED signature"), exitCode: 1}, nil
		case strings.Contains(joined, "wrong-root") && strings.HasSuffix(joined, " update"):
			return commandResult{stderr: []byte("UNTRUSTED signature"), exitCode: 1}, nil
		case strings.HasSuffix(joined, " update"):
			return commandResult{stdout: []byte("OK: 1 distinct packages available")}, nil
		default:
			return commandResult{}, nil
		}
	}}
}
