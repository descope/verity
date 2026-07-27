package chartgen

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImageDefinitionNames_returns_only_top_level_yaml_stems(t *testing.T) {
	// Given an images directory with image definitions and unrelated entries.
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "node.yaml"), []byte("contents: {}\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("ignored\n"), 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "_base"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "_base", "base.yaml"), []byte("ignored\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "postgres.yaml"), []byte("contents: {}\n"), 0o600))

	// When image definition names are loaded.
	names, err := ImageDefinitionNames(dir)

	// Then only top-level YAML filename stems are returned.
	require.NoError(t, err)
	assert.Equal(t, map[string]struct{}{"node": {}, "postgres": {}}, names)
}

func TestImageDefinitionNames_rejects_missing_directory(t *testing.T) {
	// Given a missing images directory.
	missing := filepath.Join(t.TempDir(), "missing")

	// When image definition names are loaded.
	_, err := ImageDefinitionNames(missing)

	// Then publication fails closed instead of silently dropping exclusions.
	require.Error(t, err)
}
