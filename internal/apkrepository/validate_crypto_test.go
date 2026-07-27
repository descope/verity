package apkrepository

import (
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteRepositoriesFile_uses_absolute_client_repository_path(t *testing.T) {
	// Given a fresh apk client root outside the process root filesystem.
	root := filepath.Join(t.TempDir(), "client-root")
	path := filepath.Join(t.TempDir(), "repositories")

	// When its repositories file is written.
	err := writeRepositoriesFile(path, root)

	// Then apk receives an absolute file URL instead of the broken file:///repo alias.
	require.NoError(t, err)
	contents, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	want := (&url.URL{Scheme: "file", Path: filepath.Join(root, "repo")}).String() + "\n"
	assert.Equal(t, want, string(contents))
}
