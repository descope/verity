package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
)

func intRunCatalogApp(t *testing.T, args []string) error {
	t.Helper()
	app := &cli.App{Commands: []*cli.Command{IntegerCommand}}
	return app.Run(append([]string{"verity", "integer"}, args...))
}

func TestIntegerCatalogCommand_Basic(t *testing.T) {
	imagesDir, cfgPath := intSetupCmdImages(t)
	outputPath := filepath.Join(t.TempDir(), "catalog.json")

	err := intRunCatalogApp(t, []string{
		"catalog",
		"--config", cfgPath,
		"--images-dir", imagesDir,
		"--apkindex-url", "",
		"--output", outputPath,
	})
	require.NoError(t, err)

	data, err := os.ReadFile(outputPath)
	require.NoError(t, err)

	var cat map[string]any
	require.NoError(t, json.Unmarshal(data, &cat))
	images, ok := cat["images"].([]any)
	require.True(t, ok)
	assert.Len(t, images, 1)
}

func TestIntegerCatalogCommand_WithAPKINDEX(t *testing.T) {
	srv := intMakeAPKINDEXServer(t, "P:nodejs-22\nV:22.0.0\n\nP:nodejs-24\nV:24.0.0\n\n")

	imagesDir, cfgPath := intSetupCmdImages(t)
	outputPath := filepath.Join(t.TempDir(), "catalog.json")

	err := intRunCatalogApp(t, []string{
		"catalog",
		"--config", cfgPath,
		"--images-dir", imagesDir,
		"--apkindex-url", srv.URL,
		"--cache-dir", t.TempDir(),
		"--output", outputPath,
	})
	require.NoError(t, err)
}

func TestIntegerCatalogCommand_StdoutOutput(t *testing.T) {
	imagesDir, cfgPath := intSetupCmdImages(t)

	err := intRunCatalogApp(t, []string{
		"catalog",
		"--config", cfgPath,
		"--images-dir", imagesDir,
		"--apkindex-url", "",
		"--output", "-",
	})
	require.NoError(t, err)
}

func TestIntegerCatalogCommand_InvalidConfig(t *testing.T) {
	dir := t.TempDir()
	err := intRunCatalogApp(t, []string{
		"catalog",
		"--config", filepath.Join(dir, "nonexistent.yaml"),
		"--images-dir", dir,
		"--output", filepath.Join(dir, "out.json"),
	})
	require.Error(t, err)
}
