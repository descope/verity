package sitepublication

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReplaceSiteDirectory_replaces_existing_output_and_removes_backup(t *testing.T) {
	// Given
	root := t.TempDir()
	output := filepath.Join(root, "site")
	stage := filepath.Join(root, "stage")
	writeSiteFile(t, output, "old.txt", "old")
	writeSiteFile(t, stage, "new.txt", "new")

	// When
	err := replaceSiteDirectory(stage, output)

	// Then
	require.NoError(t, err)
	assert.NoFileExists(t, filepath.Join(output, "old.txt"))
	assert.Equal(t, "new", readSiteFile(t, output, "new.txt"))
	assert.NoDirExists(t, output+".previous")
}

func TestReplaceSiteDirectory_restores_existing_output_when_stage_publish_fails(t *testing.T) {
	// Given
	root := t.TempDir()
	output := filepath.Join(root, "site")
	writeSiteFile(t, output, "old.txt", "old")
	missingStage := filepath.Join(root, "missing-stage")

	// When
	err := replaceSiteDirectory(missingStage, output)

	// Then
	require.ErrorContains(t, err, "publish assembled site")
	assert.Equal(t, "old", readSiteFile(t, output, "old.txt"))
	assert.NoDirExists(t, output+".previous")
}

func TestReplaceSiteDirectory_fails_closed_when_stage_and_output_are_missing(t *testing.T) {
	// Given
	root := t.TempDir()
	output := filepath.Join(root, "site")
	missingStage := filepath.Join(root, "missing-stage")

	// When
	err := replaceSiteDirectory(missingStage, output)

	// Then
	require.ErrorContains(t, err, "publish assembled site")
	assert.NoDirExists(t, output)
}

func TestPrepareSiteStage_and_writeSiteBytes_report_filesystem_shape_errors(t *testing.T) {
	t.Run("stage parent is a file", func(t *testing.T) {
		// Given
		root := t.TempDir()
		parent := filepath.Join(root, "parent")
		require.NoError(t, os.WriteFile(parent, []byte("sentinel"), 0o600))

		// When
		stage, err := prepareSiteStage(filepath.Join(parent, "site"))

		// Then
		require.ErrorContains(t, err, "create site output parent")
		assert.Empty(t, stage)
	})

	t.Run("metadata parent is a file", func(t *testing.T) {
		// Given
		root := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(root, "metadata"), []byte("sentinel"), 0o600))

		// When
		err := writeSiteBytes(root, "metadata/value.json", []byte("value"), 0o600)

		// Then
		require.ErrorContains(t, err, "create metadata directory")
	})

	t.Run("metadata target is a directory", func(t *testing.T) {
		// Given
		root := t.TempDir()
		require.NoError(t, os.Mkdir(filepath.Join(root, "value.json"), 0o755))

		// When
		err := writeSiteBytes(root, "value.json", []byte("value"), 0o600)

		// Then
		require.ErrorContains(t, err, "write site metadata")
	})
}
