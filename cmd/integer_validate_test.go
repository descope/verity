package cmd

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
)

func TestIntegerValidateCommand_AllValid(t *testing.T) {
	imagesDir, cfgPath := intSetupCmdImages(t)

	app := &cli.App{Commands: []*cli.Command{IntegerCommand}}
	err := app.Run([]string{
		"verity", "integer", "validate",
		"--config", cfgPath,
		"--images-dir", imagesDir,
	})
	assert.NoError(t, err)
}

func TestIntegerValidateCommand_InvalidImageYaml(t *testing.T) {
	imagesDir, cfgPath := intSetupCmdImages(t)
	intWriteFile(t, filepath.Join(imagesDir, "broken.yaml"), "not: valid: yaml: [")

	app := &cli.App{Commands: []*cli.Command{IntegerCommand}}
	err := app.Run([]string{
		"verity", "integer", "validate",
		"--config", cfgPath,
		"--images-dir", imagesDir,
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, errIntegerValidationFailed)
}

func TestIntegerValidateCommand_InvalidConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "integer.yaml")
	intWriteFile(t, cfgPath, ":: bad yaml ::")

	imagesDir := filepath.Join(dir, "images")
	intWriteFile(t, filepath.Join(imagesDir, "node.yaml"), intTestNodeYAML)

	app := &cli.App{Commands: []*cli.Command{IntegerCommand}}
	err := app.Run([]string{
		"verity", "integer", "validate",
		"--config", cfgPath,
		"--images-dir", imagesDir,
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, errIntegerValidationFailed)
}

func TestIntegerValidateCommand_APKINDEXCheck_Missing(t *testing.T) {
	srv := intMakeAPKINDEXServer(t, "P:curl\nV:8.0.0\n\n")
	imagesDir, cfgPath := intSetupCmdImages(t)

	app := &cli.App{Commands: []*cli.Command{IntegerCommand}}
	err := app.Run([]string{
		"verity", "integer", "validate",
		"--config", cfgPath,
		"--images-dir", imagesDir,
		"--apkindex-url", srv.URL,
		"--cache-dir", t.TempDir(),
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, errIntegerValidationFailed)
}

func TestIntegerValidateCommand_APKINDEXCheck_Found(t *testing.T) {
	srv := intMakeAPKINDEXServer(t, "P:nodejs-22\nV:22.0.0\n\n")
	imagesDir, cfgPath := intSetupCmdImages(t)

	app := &cli.App{Commands: []*cli.Command{IntegerCommand}}
	err := app.Run([]string{
		"verity", "integer", "validate",
		"--config", cfgPath,
		"--images-dir", imagesDir,
		"--apkindex-url", srv.URL,
		"--cache-dir", t.TempDir(),
	})
	assert.NoError(t, err)
}

func TestIntegerValidateCommand_SkipsNonYAML(t *testing.T) {
	imagesDir, cfgPath := intSetupCmdImages(t)
	intWriteFile(t, filepath.Join(imagesDir, "README.md"), "# readme")

	app := &cli.App{Commands: []*cli.Command{IntegerCommand}}
	err := app.Run([]string{
		"verity", "integer", "validate",
		"--config", cfgPath,
		"--images-dir", imagesDir,
	})
	assert.NoError(t, err)
}
