package cmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

func TestIntegerBuildCommandFindsNestedDefinitionByDeclaredName(t *testing.T) {
	// Given: a nested definition whose file path and declared name differ.
	const yamlBody = `
name: renamed-image
upstream:
  package: renamed-image
types:
  default:
    base: wolfi-base
    packages: [renamed-image]
versions:
  "1": {}
`
	dir := t.TempDir()
	imagesDir := filepath.Join(dir, "images")
	require.NoError(t, os.MkdirAll(filepath.Join(imagesDir, "_base"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(imagesDir, "_base", "wolfi-base.yaml"), []byte("# base\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(imagesDir, "platform"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(imagesDir, "platform", "custom-file.yaml"), []byte(yamlBody), 0o644))
	capture := filepath.Join(dir, "captured.apko.yaml")
	intCapturingApko(t, capture)

	// When: the CLI builds by declared image name.
	root := &cli.Command{Commands: []*cli.Command{IntegerCommand}}
	err := root.Run(context.Background(), []string{
		"verity", "integer", "build",
		"--image", "renamed-image",
		"--version", "1",
		"--type", "default",
		"--images-dir", imagesDir,
		"--apkindex-url", "",
		"--output", filepath.Join(dir, "image.tar"),
	})

	// Then: the nested definition is rendered and passed to apko.
	require.NoError(t, err)
	rendered, err := os.ReadFile(capture)
	require.NoError(t, err)
	require.Contains(t, string(rendered), "renamed-image")
}
