package apkrepository

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

var errUnexpectedCommand = errors.New("unexpected command")

type fakeCommandRunner struct {
	calls []command
	run   func(command) (commandResult, error)
}

func (runner *fakeCommandRunner) Run(_ context.Context, request *command) (commandResult, error) {
	runner.calls = append(runner.calls, *request)
	if runner.run == nil {
		return commandResult{}, fmt.Errorf("%w: %s %s", errUnexpectedCommand, request.name, strings.Join(request.args, " "))
	}
	return runner.run(*request)
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

func writeTestAPK(t *testing.T, path, name, version, architecture, payload, signature string) {
	t.Helper()
	var apk bytes.Buffer
	if signature != "" {
		apk.Write(testTarGzip(t, map[string]string{".SIGN.RSA256.test.rsa.pub": signature}))
	}
	pkgInfo := fmt.Sprintf("pkgname = %s\npkgver = %s\narch = %s\nsize = %d\n", name, version, architecture, len(payload))
	apk.Write(testTarGzip(t, map[string]string{".PKGINFO": pkgInfo}))
	apk.Write(testTarGzip(t, map[string]string{"usr/bin/" + name: payload}))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, apk.Bytes(), 0o644))
}

func testTarGzip(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	tarWriter := tar.NewWriter(gzipWriter)
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		contents := entries[name]
		require.NoError(t, tarWriter.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(contents))}))
		_, err := tarWriter.Write([]byte(contents))
		require.NoError(t, err)
	}
	require.NoError(t, tarWriter.Close())
	require.NoError(t, gzipWriter.Close())
	return buffer.Bytes()
}
