package apkrepository

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSnapshot_publishes_complete_dual_architecture_set(t *testing.T) {
	// Given a complete x86_64/aarch64 package set and an existing unrelated file.
	root := t.TempDir()
	source := filepath.Join(root, "source")
	output := filepath.Join(root, "output")
	writeTestAPK(t, filepath.Join(source, "x86_64", "demo-1.0-r0.apk"), "demo", "1.0-r0", "x86_64", "x86", "")
	writeTestAPK(t, filepath.Join(source, "aarch64", "demo-1.0-r0.apk"), "demo", "1.0-r0", "aarch64", "arm", "")
	writeTestFile(t, filepath.Join(output, "index.html"), "docs")
	options := signedSnapshotOptions(t, source, output)
	runner := snapshotRunner(t)
	options.runner = runner
	var stdout bytes.Buffer
	options.Stdout = &stdout

	// When the snapshot transition is applied.
	err := Snapshot(context.Background(), options)

	// Then both architectures are signed and indexed with Melange while unrelated bytes survive.
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(output, "x86_64", "demo-1.0-r0.apk"))
	assert.FileExists(t, filepath.Join(output, "aarch64", "demo-1.0-r0.apk"))
	assert.FileExists(t, filepath.Join(output, "x86_64", "APKINDEX.tar.gz"))
	assert.FileExists(t, filepath.Join(output, "aarch64", "APKINDEX.tar.gz"))
	assert.FileExists(t, filepath.Join(output, "index.html"))
	assert.Contains(t, stdout.String(), filepath.Join(output, "verity.rsa.pub"))
	assert.NotContains(t, stdout.String(), ".stage-")
	require.Len(t, runner.calls, 4)
	for _, call := range runner.calls {
		assert.Equal(t, "melange", call.name)
	}
}

func TestSnapshot_rejects_incomplete_set_before_mutating_output(t *testing.T) {
	// Given a snapshot missing aarch64 and pre-existing output bytes.
	root := t.TempDir()
	source := filepath.Join(root, "source")
	output := filepath.Join(root, "output")
	writeTestAPK(t, filepath.Join(source, "x86_64", "demo.apk"), "demo", "1.0-r0", "x86_64", "payload", "")
	marker := filepath.Join(output, "published.txt")
	writeTestFile(t, marker, "unchanged")
	options := signedSnapshotOptions(t, source, output)

	// When the incomplete snapshot is requested.
	err := Snapshot(context.Background(), options)

	// Then it fails closed without touching prior publication bytes.
	require.ErrorIs(t, err, errSnapshotIncomplete)
	contents, readErr := os.ReadFile(marker)
	require.NoError(t, readErr)
	assert.Equal(t, "unchanged", string(contents))
}

func TestSnapshot_publishes_traversable_root_when_output_is_new(t *testing.T) {
	// Given a complete snapshot and an output path that does not exist.
	root := t.TempDir()
	source := filepath.Join(root, "source")
	output := filepath.Join(root, "output")
	writeTestAPK(t, filepath.Join(source, "x86_64", "demo.apk"), "demo", "1.0-r0", "x86_64", "x86", "")
	writeTestAPK(t, filepath.Join(source, "aarch64", "demo.apk"), "demo", "1.0-r0", "aarch64", "arm", "")
	options := signedSnapshotOptions(t, source, output)
	options.runner = snapshotRunner(t)

	// When the snapshot is published through a staged rename.
	err := Snapshot(context.Background(), options)

	// Then fresh clients can traverse the repository root.
	require.NoError(t, err)
	info, statErr := os.Stat(output)
	require.NoError(t, statErr)
	assert.Equal(t, os.FileMode(0o755), info.Mode().Perm())
}

func TestSnapshot_rejects_duplicate_architecture_package_name(t *testing.T) {
	// Given two x86_64 versions for one package key and a complete second architecture.
	root := t.TempDir()
	first := filepath.Join(root, "first")
	second := filepath.Join(root, "second")
	writeTestAPK(t, filepath.Join(first, "x86_64", "demo-1.0-r0.apk"), "demo", "1.0-r0", "x86_64", "first", "")
	writeTestAPK(t, filepath.Join(second, "x86_64", "demo-2.0-r0.apk"), "demo", "2.0-r0", "x86_64", "second", "")
	writeTestAPK(t, filepath.Join(first, "aarch64", "demo.apk"), "demo", "1.0-r0", "aarch64", "arm", "")
	options := signedSnapshotOptions(t, first, filepath.Join(root, "output"))
	options.Sources = []string{first, second}

	// When the snapshot is parsed.
	err := Snapshot(context.Background(), options)

	// Then the arch+package-name upsert key cannot be ambiguous.
	require.ErrorIs(t, err, errDuplicatePackageKey)
}

func signedSnapshotOptions(t *testing.T, source, output string) *SnapshotOptions {
	t.Helper()
	privatePEM, publicPEM := testRSAKeyPair(t)
	publicKey := filepath.Join(t.TempDir(), "verity.rsa.pub")
	require.NoError(t, os.WriteFile(publicKey, publicPEM, 0o644))
	return &SnapshotOptions{
		OutputDir: output, PublicKeyPath: publicKey, Sources: []string{source},
		PrivateKeyPEM: privatePEM,
	}
}

func snapshotRunner(t *testing.T) *fakeCommandRunner {
	t.Helper()
	return &fakeCommandRunner{run: func(request command) (commandResult, error) {
		if request.name != "melange" || len(request.args) == 0 {
			return commandResult{}, fmt.Errorf("%w: %s", errUnexpectedCommand, request.name)
		}
		switch request.args[0] {
		case "sign":
			return commandResult{}, nil
		case "index":
			writeTestIndex(t, filepath.Join(request.dir, "APKINDEX.tar.gz"), ".SIGN.RSA256.verity.rsa.pub", "APKINDEX")
			return commandResult{}, nil
		default:
			return commandResult{}, fmt.Errorf("%w: melange %s", errUnexpectedCommand, request.args[0])
		}
	}}
}
