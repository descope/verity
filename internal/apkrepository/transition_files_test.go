package apkrepository

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrepareStagedOutput_recovers_interrupted_publication_backup(t *testing.T) {
	// Given a hard interruption after the prior output was renamed to its backup.
	output := filepath.Join(t.TempDir(), "repository")
	backupFile := filepath.Join(output+".previous", "published.txt")
	writeTestFile(t, backupFile, "published bytes")

	// When the next transition prepares its staging directory.
	stage, err := prepareStagedOutput(output)
	require.NoError(t, err)
	defer stage.cleanup()

	// Then the prior publication is restored before any new work starts.
	assert.Equal(t, "published bytes", string(mustReadFile(t, filepath.Join(output, "published.txt"))))
	assert.Equal(t, "published bytes", string(mustReadFile(t, filepath.Join(stage.path, "published.txt"))))
	assert.NoDirExists(t, output+".previous")
	info, statErr := os.Stat(output)
	require.NoError(t, statErr)
	assert.Equal(t, os.FileMode(0o755), info.Mode().Perm())
}
