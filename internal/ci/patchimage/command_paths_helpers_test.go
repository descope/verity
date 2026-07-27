package patchimage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func useTempWorkingDirectory(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	previous, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(directory))
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(previous))
	})
	return directory
}

func installFakeExecutable(t *testing.T, directory, name, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(directory, 0o700))
	content := "#!/bin/sh\nset -eu\n" + body + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(directory, name), []byte(content), 0o700))
}

func prependCommandPath(t *testing.T, directory string) {
	t.Helper()
	t.Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func readMetricsDocument(t *testing.T, path string) MetricsDocument {
	t.Helper()
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	var document MetricsDocument
	require.NoError(t, json.Unmarshal(content, &document))
	return document
}

func readTextFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(content)
}
