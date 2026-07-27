package apkrepository

import (
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSnapshot_rejects_malicious_APK_inputs_before_signing(t *testing.T) {
	tests := []struct {
		name  string
		write func(*testing.T, string)
	}{
		{name: "malformed archive", write: func(t *testing.T, path string) {
			require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
			require.NoError(t, os.WriteFile(path, []byte("not-an-apk"), 0o644))
		}},
		{name: "unsafe package path", write: func(t *testing.T, path string) {
			writeTestAPK(t, path, "../escape", "1.0-r0", "x86_64", "payload", "")
		}},
		{name: "metadata decompression bomb", write: writeOversizedAPKMetadata},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given one hostile x86_64 package and one otherwise valid architecture peer.
			root := t.TempDir()
			source := filepath.Join(root, "source")
			test.write(t, filepath.Join(source, "x86_64", "hostile.apk"))
			writeTestAPK(t, filepath.Join(source, "aarch64", "demo.apk"), "demo", "1.0-r0", "aarch64", "payload", "")
			options := signedSnapshotOptions(t, source, filepath.Join(root, "output"))
			runner := &fakeCommandRunner{}
			options.runner = runner

			// When the snapshot boundary inspects inputs.
			err := Snapshot(context.Background(), options)

			// Then the hostile archive is rejected before any signing command runs.
			require.Error(t, err)
			assert.Empty(t, runner.calls)
			assert.NoDirExists(t, options.OutputDir)
		})
	}
}

func writeOversizedAPKMetadata(t *testing.T, path string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	file, err := os.Create(path)
	require.NoError(t, err)
	writer := gzip.NewWriter(file)
	_, copyErr := io.CopyN(writer, zeroReader{}, maxAPKMetadataSize+1)
	closeWriterErr := writer.Close()
	closeFileErr := file.Close()
	require.NoError(t, copyErr)
	require.NoError(t, closeWriterErr)
	require.NoError(t, closeFileErr)
}

type zeroReader struct{}

func (zeroReader) Read(buffer []byte) (int, error) {
	clear(buffer)
	return len(buffer), nil
}
