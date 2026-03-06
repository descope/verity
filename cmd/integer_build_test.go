package cmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"

	intconfig "github.com/verity-org/verity/internal/integer/config"
)

const intBuildNodeYAML = `
name: myapp
upstream:
  package: myapp
types:
  default:
    base: wolfi-base
    packages: [myapp]
    entrypoint: /usr/bin/myapp
versions:
  latest:
    latest: true
`

func intSetupBuildImages(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	imagesDir := filepath.Join(dir, "images")
	baseDir := filepath.Join(imagesDir, "_base")
	require.NoError(t, os.MkdirAll(baseDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(baseDir, "wolfi-base.yaml"), []byte("# base\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(imagesDir, "myapp.yaml"), []byte(intBuildNodeYAML), 0o644))
	return imagesDir
}

func intFakeApko(t *testing.T, exitCode int) {
	t.Helper()
	tmpDir := t.TempDir()
	script := filepath.Join(tmpDir, "apko")
	content := "#!/bin/sh\nexit " + string(rune('0'+exitCode)) + "\n"
	require.NoError(t, os.WriteFile(script, []byte(content), 0o755))
	existing := os.Getenv("PATH")
	t.Setenv("PATH", tmpDir+":"+existing)
}

func TestIntegerBuildCommand_UnknownType(t *testing.T) {
	imagesDir := intSetupBuildImages(t)

	app := &cli.App{Commands: []*cli.Command{IntegerCommand}}
	err := app.Run([]string{
		"verity", "integer", "build",
		"--image", "myapp",
		"--version", "latest",
		"--type", "jre",
		"--images-dir", imagesDir,
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, errIntegerVariantNotFound)
}

func TestIntegerBuildCommand_MissingImage(t *testing.T) {
	imagesDir := intSetupBuildImages(t)

	app := &cli.App{Commands: []*cli.Command{IntegerCommand}}
	err := app.Run([]string{
		"verity", "integer", "build",
		"--image", "nonexistent",
		"--version", "latest",
		"--images-dir", imagesDir,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nonexistent")
}

func TestIntegerRunApkoBuild_NotInPath(t *testing.T) {
	t.Setenv("PATH", "")
	err := integerRunApkoBuild(context.Background(), "config.yaml", "out.tar", "amd64")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "apko not found")
}

func TestIntegerRunApkoBuild_Fails(t *testing.T) {
	intFakeApko(t, 1)
	err := integerRunApkoBuild(context.Background(), "config.yaml", "out.tar", "amd64")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "apko build failed")
}

func TestIntegerRunApkoBuild_Success(t *testing.T) {
	intFakeApko(t, 0)
	err := integerRunApkoBuild(context.Background(), "config.yaml", "out.tar", "amd64")
	require.NoError(t, err)
}

func TestIntegerResolveLatestVersion_Success(t *testing.T) {
	srv := intMakeAPKINDEXServer(t, "P:nodejs-22\nV:22.0.0\n\nP:nodejs-24\nV:24.0.0\n\n")

	def := &intconfig.ImageDef{
		Name:     "node",
		Upstream: intconfig.Upstream{Package: "nodejs-{{version}}"},
	}

	v, err := integerResolveLatestVersion(def, srv.URL)
	require.NoError(t, err)
	assert.Equal(t, "24", v)
}

func TestIntegerResolveLatestVersion_NoVersions(t *testing.T) {
	srv := intMakeAPKINDEXServer(t, "P:curl\nV:8.0.0\n\n")

	def := &intconfig.ImageDef{
		Name:     "node",
		Upstream: intconfig.Upstream{Package: "nodejs-{{version}}"},
	}

	_, err := integerResolveLatestVersion(def, srv.URL)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no versions found")
}
