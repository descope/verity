package cmd

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

func TestIntegerValidateCommand_AllValid(t *testing.T) {
	imagesDir, cfgPath := intSetupCmdImages(t)

	root := &cli.Command{Commands: []*cli.Command{IntegerCommand}}
	err := root.Run(context.Background(), []string{
		"verity", "integer", "validate",
		"--config", cfgPath,
		"--images-dir", imagesDir,
	})
	assert.NoError(t, err)
}

func TestIntegerValidateCommand_InvalidImageYaml(t *testing.T) {
	imagesDir, cfgPath := intSetupCmdImages(t)
	intWriteFile(t, filepath.Join(imagesDir, "broken.yaml"), "not: valid: yaml: [")

	root := &cli.Command{Commands: []*cli.Command{IntegerCommand}}
	err := root.Run(context.Background(), []string{
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

	root := &cli.Command{Commands: []*cli.Command{IntegerCommand}}
	err := root.Run(context.Background(), []string{
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

	root := &cli.Command{Commands: []*cli.Command{IntegerCommand}}
	err := root.Run(context.Background(), []string{
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

	root := &cli.Command{Commands: []*cli.Command{IntegerCommand}}
	err := root.Run(context.Background(), []string{
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

	root := &cli.Command{Commands: []*cli.Command{IntegerCommand}}
	err := root.Run(context.Background(), []string{
		"verity", "integer", "validate",
		"--config", cfgPath,
		"--images-dir", imagesDir,
	})
	assert.NoError(t, err)
}
