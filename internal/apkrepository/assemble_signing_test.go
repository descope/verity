package apkrepository

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAssemble_signs_packages_and_RSA256_indexes_with_matching_key(t *testing.T) {
	// Given a package, matching keypair, and process adapter that records signing commands.
	root := t.TempDir()
	source := filepath.Join(root, "source")
	output := filepath.Join(root, "output")
	publicPath := filepath.Join(root, "verity.rsa.pub")
	writeTestFile(t, filepath.Join(source, "x86_64", "demo.apk"), "package")
	privatePEM, publicPEM := testRSAKeyPair(t)
	require.NoError(t, os.WriteFile(publicPath, publicPEM, 0o644))
	runner := repositoryBuildRunner(t)

	// When a signed repository is assembled.
	err := Assemble(context.Background(), &AssembleOptions{
		OutputDir: output, PublicKeyPath: publicPath, Sources: []string{source},
		PrivateKeyPEM: privatePEM, runner: runner,
	})

	// Then the package, format marker, public key, and RSA256 index are published.
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(output, "x86_64", "demo.apk"))
	assert.FileExists(t, filepath.Join(output, "x86_64", "APKINDEX.tar.gz"))
	assert.FileExists(t, filepath.Join(output, "verity.rsa.pub"))
	format, readErr := os.ReadFile(filepath.Join(output, "repository-format"))
	require.NoError(t, readErr)
	assert.Equal(t, "1\n", string(format))
	require.Len(t, runner.calls, 3)
	assert.Equal(t, "melange", runner.calls[0].name)
	assert.Contains(t, runner.calls[2].args, "RSA256")
}

func TestAssemble_builds_unsigned_index_when_private_key_is_absent(t *testing.T) {
	// Given one package and a fake apk index process.
	root := t.TempDir()
	source := filepath.Join(root, "source")
	output := filepath.Join(root, "output")
	writeTestFile(t, filepath.Join(source, "aarch64", "demo.apk"), "package")
	runner := repositoryBuildRunner(t)

	// When a local unsigned repository is assembled.
	err := Assemble(context.Background(), &AssembleOptions{
		OutputDir: output, Sources: []string{source}, runner: runner,
	})

	// Then apk receives the explicit local-only trust mode without signing commands.
	require.NoError(t, err)
	require.Len(t, runner.calls, 1)
	assert.Equal(t, "apk", runner.calls[0].name)
	assert.Contains(t, runner.calls[0].args, "--allow-untrusted")
}

func repositoryBuildRunner(t *testing.T) *fakeCommandRunner {
	t.Helper()
	return &fakeCommandRunner{run: func(request command) (commandResult, error) {
		switch request.name {
		case "melange":
			packagePath := request.args[len(request.args)-1]
			file, err := os.OpenFile(packagePath, os.O_APPEND|os.O_WRONLY, 0)
			if err != nil {
				return commandResult{}, err
			}
			_, writeErr := file.WriteString("-signed")
			return commandResult{}, errors.Join(writeErr, file.Close())
		case "apk":
			writeTestFile(t, filepath.Join(request.dir, "APKINDEX.tar.gz"), "index")
			return commandResult{}, nil
		case "abuild-sign":
			return commandResult{}, nil
		default:
			return commandResult{}, assert.AnError
		}
	}}
}
