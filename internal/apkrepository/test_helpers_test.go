package apkrepository

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

var errUnexpectedCommand = errors.New("unexpected command")

type fakeCommandRunner struct {
	calls []command
	run   func(command) (commandResult, error)
}

func (runner *fakeCommandRunner) Run(_ context.Context, request command) (commandResult, error) {
	runner.calls = append(runner.calls, request)
	if runner.run == nil {
		return commandResult{}, fmt.Errorf("%w: %s %s", errUnexpectedCommand, request.name, strings.Join(request.args, " "))
	}
	return runner.run(request)
}

func writeTestFile(t *testing.T, path, contents string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o644))
}

func writeTestIndex(t *testing.T, path string, names ...string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	file, err := os.Create(path)
	require.NoError(t, err)
	gzipWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzipWriter)
	for _, name := range names {
		require.NoError(t, tarWriter.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: 0}))
	}
	require.NoError(t, tarWriter.Close())
	require.NoError(t, gzipWriter.Close())
	require.NoError(t, file.Close())
}

func pagesArtifactZip(t *testing.T, tarEntries map[string]string) []byte {
	t.Helper()
	var tarBuffer bytes.Buffer
	tarWriter := tar.NewWriter(&tarBuffer)
	for name, contents := range tarEntries {
		require.NoError(t, tarWriter.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(contents))}))
		_, err := tarWriter.Write([]byte(contents))
		require.NoError(t, err)
	}
	require.NoError(t, tarWriter.Close())
	var zipBuffer bytes.Buffer
	zipWriter := zip.NewWriter(&zipBuffer)
	artifact, err := zipWriter.Create("artifact.tar")
	require.NoError(t, err)
	_, err = artifact.Write(tarBuffer.Bytes())
	require.NoError(t, err)
	require.NoError(t, zipWriter.Close())
	return zipBuffer.Bytes()
}
