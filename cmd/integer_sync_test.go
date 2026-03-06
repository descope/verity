package cmd

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"

	intconfig "github.com/verity-org/verity/internal/integer/config"
)

func intRunSyncApp(t *testing.T, args []string) error {
	t.Helper()
	app := &cli.App{Commands: []*cli.Command{IntegerCommand}}
	return app.Run(append([]string{"verity", "integer"}, args...))
}

func intMakeAPKINDEXServer(t *testing.T, content string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		tw := tar.NewWriter(gz)
		data := []byte(content)
		if err := tw.WriteHeader(&tar.Header{Name: "APKINDEX", Mode: 0o644, Size: int64(len(data))}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if _, err := tw.Write(data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		tw.Close()
		gz.Close()
		if _, err := w.Write(buf.Bytes()); err != nil {
			return
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestIntegerSyncCommand_UpToDate(t *testing.T) {
	dir := t.TempDir()
	imagesDir := filepath.Join(dir, "images")

	const curlYAML = `
name: curl
upstream:
  package: curl
types:
  default:
    base: wolfi-base
    packages: [curl]
    entrypoint: /usr/bin/curl
versions:
  latest:
    latest: true
`
	intWriteFile(t, filepath.Join(imagesDir, "_base", "wolfi-base.yaml"), "# base\n")
	intWriteFile(t, filepath.Join(imagesDir, "curl.yaml"), curlYAML)

	err := intRunSyncApp(t, []string{
		"sync",
		"--images-dir", imagesDir,
		"--apkindex-url", "",
	})
	require.NoError(t, err)
}

func TestIntegerSyncCommand_WithAPKINDEX_NewVersion(t *testing.T) {
	srv := intMakeAPKINDEXServer(t, "P:nodejs-22\nV:22.0.0\n\nP:nodejs-24\nV:24.0.0\n\n")

	dir := t.TempDir()
	imagesDir := filepath.Join(dir, "images")
	intWriteFile(t, filepath.Join(imagesDir, "_base", "wolfi-base.yaml"), "# base\n")
	intWriteFile(t, filepath.Join(imagesDir, "node.yaml"), intTestNodeYAML)

	err := intRunSyncApp(t, []string{
		"sync",
		"--images-dir", imagesDir,
		"--apkindex-url", srv.URL,
		"--cache-dir", t.TempDir(),
	})
	require.NoError(t, err)
}

func TestIntegerSyncCommand_Apply(t *testing.T) {
	srv := intMakeAPKINDEXServer(t, "P:nodejs-22\nV:22.0.0\n\nP:nodejs-24\nV:24.0.0\n\n")

	dir := t.TempDir()
	imagesDir := filepath.Join(dir, "images")
	intWriteFile(t, filepath.Join(imagesDir, "_base", "wolfi-base.yaml"), "# base\n")
	intWriteFile(t, filepath.Join(imagesDir, "node.yaml"), intTestNodeYAML)

	err := intRunSyncApp(t, []string{
		"sync",
		"--images-dir", imagesDir,
		"--apkindex-url", srv.URL,
		"--cache-dir", t.TempDir(),
		"--apply",
	})
	require.NoError(t, err)

	updated, err := intconfig.LoadImage(filepath.Join(imagesDir, "node.yaml"))
	require.NoError(t, err)
	assert.Contains(t, updated.Versions, "22")
	assert.Contains(t, updated.Versions, "24")
}

func TestIntegerSyncCommand_MissingImagesDir(t *testing.T) {
	err := intRunSyncApp(t, []string{
		"sync",
		"--images-dir", "/nonexistent/images",
		"--apkindex-url", "",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading images directory")
}

func TestIntegerApplySyncUpdates(t *testing.T) {
	dir := t.TempDir()
	imagesDir := filepath.Join(dir, "images")

	const imageYAML = `
name: node
upstream:
  package: "nodejs-{{version}}"
types:
  default:
    base: wolfi-base
    packages: ["nodejs-{{version}}"]
    entrypoint: /usr/bin/node
versions:
  "22": {}
`
	intWriteFile(t, filepath.Join(imagesDir, "_base", "wolfi-base.yaml"), "# base\n")
	intWriteFile(t, filepath.Join(imagesDir, "node.yaml"), imageYAML)

	imagePath := filepath.Join(imagesDir, "node.yaml")
	def, err := intconfig.LoadImage(imagePath)
	require.NoError(t, err)

	err = integerApplySyncUpdates(imagePath, def, []string{"24", "26"})
	require.NoError(t, err)

	updated, err := intconfig.LoadImage(imagePath)
	require.NoError(t, err)
	assert.Contains(t, updated.Versions, "22")
	assert.Contains(t, updated.Versions, "24")
	assert.Contains(t, updated.Versions, "26")
}
