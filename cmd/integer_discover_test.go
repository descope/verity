package cmd

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
)

const intTestIntegerYAML = `
target:
  registry: ghcr.io/test-org
defaults:
  archs: [amd64, arm64]
`

const intTestNodeYAML = `
name: node
description: "Node.js"
upstream:
  package: "nodejs-{{version}}"
types:
  default:
    base: wolfi-base
    packages: ["nodejs-{{version}}"]
    entrypoint: /usr/bin/node
  dev:
    base: wolfi-dev
    packages: ["nodejs-{{version}}", "npm"]
    entrypoint: /usr/bin/node
versions:
  "22": {}
`

func intWriteFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func intSetupCmdImages(t *testing.T) (imagesDir, cfgPath string) {
	t.Helper()
	dir := t.TempDir()
	cfgPath = filepath.Join(dir, "integer.yaml")
	intWriteFile(t, cfgPath, intTestIntegerYAML)

	imagesDir = filepath.Join(dir, "images")
	intWriteFile(t, filepath.Join(imagesDir, "_base", "wolfi-base.yaml"), "# base\n")
	intWriteFile(t, filepath.Join(imagesDir, "_base", "wolfi-dev.yaml"), "# base\n")
	intWriteFile(t, filepath.Join(imagesDir, "_base", "wolfi-fips.yaml"), "# base\n")
	intWriteFile(t, filepath.Join(imagesDir, "node.yaml"), intTestNodeYAML)
	return imagesDir, cfgPath
}

func TestIntegerDiscoverCommand(t *testing.T) {
	imagesDir, cfgPath := intSetupCmdImages(t)
	genDir := t.TempDir()

	app := &cli.App{Commands: []*cli.Command{IntegerCommand}}

	r, w, err := os.Pipe()
	require.NoError(t, err)

	origStdout := os.Stdout
	os.Stdout = w

	runErr := app.Run([]string{
		"verity", "integer", "discover",
		"--config", cfgPath,
		"--images-dir", imagesDir,
		"--apkindex-url", "",
		"--gen-dir", genDir,
	})

	w.Close()
	os.Stdout = origStdout

	require.NoError(t, runErr)

	out, err := io.ReadAll(r)
	require.NoError(t, err)

	var captured []map[string]any
	require.NoError(t, json.Unmarshal(out, &captured))

	require.Len(t, captured, 2)

	types := make([]string, 0, len(captured))
	for _, entry := range captured {
		v, ok := entry["type"].(string)
		require.True(t, ok)
		types = append(types, v)
	}
	assert.ElementsMatch(t, []string{"default", "dev"}, types)
	assert.Equal(t, "ghcr.io/test-org", captured[0]["registry"])
}

func TestIntegerDiscoverCommand_MissingConfig(t *testing.T) {
	app := &cli.App{Commands: []*cli.Command{IntegerCommand}}
	err := app.Run([]string{"verity", "integer", "discover", "--config", "/nonexistent/integer.yaml"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "loading config")
}

func TestIntegerDiscoverCommand_MissingImagesDir(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "integer.yaml")
	intWriteFile(t, cfgPath, intTestIntegerYAML)

	app := &cli.App{Commands: []*cli.Command{IntegerCommand}}
	err := app.Run([]string{
		"verity", "integer", "discover",
		"--config", cfgPath,
		"--images-dir", "/nonexistent/images",
		"--apkindex-url", "",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "discovering images")
}
