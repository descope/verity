package patchimage

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOtelInstaller_verifiesDigestExtractsBinaryAndAppendsPath(t *testing.T) {
	// Given
	archive := otelArchive(t, []byte("otel-binary"))
	digest := sha256.Sum256(archive)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, err := writer.Write(archive)
		assert.NoError(t, err)
	}))
	defer server.Close()
	home := t.TempDir()
	pathFile := filepath.Join(t.TempDir(), "github-path")
	installer := OtelInstaller{Client: server.Client(), BaseURL: server.URL}

	// When
	err := installer.Install(context.Background(), &OtelInstallInput{
		Version: "0.4.5", GOOS: "linux", GOARCH: "amd64", ExpectedSHA256: hex.EncodeToString(digest[:]),
		HomeDir: home, GitHubPath: pathFile,
	})

	// Then
	require.NoError(t, err)
	binary, err := os.ReadFile(filepath.Join(home, ".local", "bin", "otel-cli"))
	require.NoError(t, err)
	assert.Equal(t, []byte("otel-binary"), binary)
	pathValue, err := os.ReadFile(pathFile)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, ".local", "bin")+"\n", string(pathValue))
}

func otelArchive(t *testing.T, binary []byte) []byte {
	t.Helper()
	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	tarWriter := tar.NewWriter(gzipWriter)
	require.NoError(t, tarWriter.WriteHeader(&tar.Header{Name: "otel-cli", Mode: 0o755, Size: int64(len(binary))}))
	_, err := tarWriter.Write(binary)
	require.NoError(t, err)
	require.NoError(t, tarWriter.Close())
	require.NoError(t, gzipWriter.Close())
	return buffer.Bytes()
}
