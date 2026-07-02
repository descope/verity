package cmd

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"

	"github.com/verity-org/verity/internal/integer/apkindex"
)

const intTestVariantYAML = `
name: distroless/static
description: "Pure static runtime"
upstream:
  package: wolfi-baselayout
types:
  default:
    base: wolfi-base
versions:
  latest:
    latest: true
  nonroot: {}
  debug: {}
`

func TestIntegerDiscoverCommand_ZeroMatchesEmitsEmptyArray(t *testing.T) {
	imagesDir, cfgPath := intSetupCmdImages(t)
	genDir := t.TempDir()

	root := &cli.Command{Commands: []*cli.Command{IntegerCommand}}

	r, w, err := os.Pipe()
	require.NoError(t, err)
	origStdout := os.Stdout
	os.Stdout = w

	runErr := root.Run(context.Background(), []string{
		"verity", "integer", "discover",
		"--config", cfgPath,
		"--images-dir", imagesDir,
		"--apkindex-url", "",
		"--gen-dir", genDir,
		"--only", "no-such-image",
	})

	w.Close()
	os.Stdout = origStdout
	require.NoError(t, runErr)

	out, err := io.ReadAll(r)
	require.NoError(t, err)

	assert.JSONEq(t, "[]", string(out))
}

func TestIntegerDiscoverCommand_NestedImageMatchedByOnly(t *testing.T) {
	imagesDir, cfgPath := intSetupCmdImages(t)
	intWriteFile(t, filepath.Join(imagesDir, "distroless", "static.yaml"), intTestVariantYAML)
	genDir := t.TempDir()

	root := &cli.Command{Commands: []*cli.Command{IntegerCommand}}

	r, w, err := os.Pipe()
	require.NoError(t, err)
	origStdout := os.Stdout
	os.Stdout = w

	runErr := root.Run(context.Background(), []string{
		"verity", "integer", "discover",
		"--config", cfgPath,
		"--images-dir", imagesDir,
		"--apkindex-url", "",
		"--gen-dir", genDir,
		"--only", "distroless/static",
	})

	w.Close()
	os.Stdout = origStdout
	require.NoError(t, runErr)

	out, err := io.ReadAll(r)
	require.NoError(t, err)

	var captured []struct {
		Name string   `json:"name"`
		Tags []string `json:"tags"`
	}
	require.NoError(t, json.Unmarshal(out, &captured))
	require.Len(t, captured, 3)
	allTags := []string{}
	for _, entry := range captured {
		assert.Equal(t, "distroless/static", entry.Name)
		allTags = append(allTags, entry.Tags...)
	}
	assert.ElementsMatch(t, []string{"latest", "nonroot", "debug"}, allTags)
}

func TestIntegerSync_VariantVersionKeysAreNotStale(t *testing.T) {
	dir := t.TempDir()
	defPath := filepath.Join(dir, "static.yaml")
	intWriteFile(t, defPath, intTestVariantYAML)

	pkgs := []apkindex.Package{{Name: "wolfi-baselayout", Version: "20260101-r0"}}

	newCount, staleCount := integerProcessSyncEntry(defPath, pkgs, false)

	assert.Equal(t, 0, newCount)
	assert.Equal(t, 0, staleCount)
}
