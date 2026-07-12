package cmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

func TestIntegerMetadataCommandFindsNestedDefinition(t *testing.T) {
	// Given: a nested definition whose file path and declared name differ.
	dir := t.TempDir()
	imagesDir := filepath.Join(dir, "images")
	intWriteFile(t, filepath.Join(imagesDir, "platform", "custom-file.yaml"), `
name: renamed-image
description: nested image description
`)
	output := filepath.Join(dir, "github-output")

	// When: metadata is emitted by declared image name.
	root := &cli.Command{Commands: []*cli.Command{IntegerCommand}}
	err := root.Run(context.Background(), []string{
		"verity", "integer", "metadata",
		"--image", "renamed-image",
		"--images-dir", imagesDir,
		"--github-output", output,
	})

	// Then: GitHub outputs contain the nested definition metadata.
	require.NoError(t, err)
	data, err := os.ReadFile(output)
	require.NoError(t, err)
	require.Equal(t, "title<<__VERITY_IMAGE_TITLE__\nrenamed-image\n__VERITY_IMAGE_TITLE__\ndescription<<__VERITY_IMAGE_DESCRIPTION__\nnested image description\n__VERITY_IMAGE_DESCRIPTION__\n", string(data))
}

func TestIntegerMetadataCommandRejectsUnknownImage(t *testing.T) {
	imagesDir := filepath.Join(t.TempDir(), "images")
	require.NoError(t, os.MkdirAll(imagesDir, 0o755))

	root := &cli.Command{Commands: []*cli.Command{IntegerCommand}}
	err := root.Run(context.Background(), []string{
		"verity", "integer", "metadata",
		"--image", "missing",
		"--images-dir", imagesDir,
		"--github-output", filepath.Join(t.TempDir(), "github-output"),
	})

	require.ErrorContains(t, err, `loading image "missing"`)
}

func TestIntegerMetadataCommandReportsOutputOpenFailure(t *testing.T) {
	dir := t.TempDir()
	imagesDir := filepath.Join(dir, "images")
	intWriteFile(t, filepath.Join(imagesDir, "node.yaml"), "name: node\n")

	root := &cli.Command{Commands: []*cli.Command{IntegerCommand}}
	err := root.Run(context.Background(), []string{
		"verity", "integer", "metadata",
		"--image", "node",
		"--images-dir", imagesDir,
		"--github-output", dir,
	})

	require.ErrorContains(t, err, "opening GitHub output")
}
