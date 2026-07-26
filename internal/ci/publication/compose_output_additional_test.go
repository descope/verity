package publication

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteComposeOutputs_publishes_exact_private_outputs_when_paths_are_distinct(t *testing.T) {
	// Given
	root := t.TempDir()
	publicationPath := filepath.Join(root, "publication.json")
	componentsPath := filepath.Join(root, "components.json")
	result := &ComposeResult{
		PublicationJSON: []byte(`{"publication":"sentinel"}`),
		ComponentsJSON:  []byte(`[{"component":"sentinel"}]`),
	}

	// When
	err := WriteComposeOutputs(publicationPath, componentsPath, result)

	// Then
	require.NoError(t, err)
	assert.Equal(t, result.PublicationJSON, readComposeOutput(t, publicationPath))
	assert.Equal(t, result.ComponentsJSON, readComposeOutput(t, componentsPath))
	for _, path := range []string{publicationPath, componentsPath} {
		info, statErr := os.Stat(path)
		require.NoError(t, statErr)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}
	staged, globErr := filepath.Glob(filepath.Join(root, ".publication-compose-*"))
	require.NoError(t, globErr)
	assert.Empty(t, staged)
}

func TestWriteComposeOutputs_rejects_ambiguous_output_contracts(t *testing.T) {
	tests := []struct {
		name            string
		result          *ComposeResult
		publicationPath string
		componentsPath  string
	}{
		{name: "nil result", result: nil, publicationPath: "publication.json", componentsPath: "components.json"},
		{name: "missing publication path", result: &ComposeResult{}, componentsPath: "components.json"},
		{name: "missing components path", result: &ComposeResult{}, publicationPath: "publication.json"},
		{name: "same path", result: &ComposeResult{}, publicationPath: "output.json", componentsPath: "output.json"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given
			root := t.TempDir()
			publicationPath := filepath.Join(root, test.publicationPath)
			componentsPath := filepath.Join(root, test.componentsPath)
			if test.publicationPath == "" {
				publicationPath = ""
			}
			if test.componentsPath == "" {
				componentsPath = ""
			}

			// When
			err := WriteComposeOutputs(publicationPath, componentsPath, test.result)

			// Then
			require.ErrorIs(t, err, ErrComposeInvalid)
			entries, readErr := os.ReadDir(root)
			require.NoError(t, readErr)
			assert.Empty(t, entries)
		})
	}
}

func TestWriteComposeOutputs_removes_staged_components_when_publication_staging_fails(t *testing.T) {
	// Given
	root := t.TempDir()
	componentsPath := filepath.Join(root, "components.json")
	publicationPath := filepath.Join(root, "missing", "publication.json")
	result := &ComposeResult{PublicationJSON: []byte("publication"), ComponentsJSON: []byte("components")}

	// When
	err := WriteComposeOutputs(publicationPath, componentsPath, result)

	// Then
	require.Error(t, err)
	assert.NoFileExists(t, componentsPath)
	staged, globErr := filepath.Glob(filepath.Join(root, ".publication-compose-*"))
	require.NoError(t, globErr)
	assert.Empty(t, staged)
}

func TestWriteComposeOutputs_reports_components_publish_failure_without_other_output(t *testing.T) {
	// Given
	root := t.TempDir()
	componentsPath := filepath.Join(root, "components.json")
	require.NoError(t, os.Mkdir(componentsPath, 0o755))
	publicationPath := filepath.Join(root, "publication.json")
	result := &ComposeResult{PublicationJSON: []byte("publication"), ComponentsJSON: []byte("components")}

	// When
	err := WriteComposeOutputs(publicationPath, componentsPath, result)

	// Then
	require.ErrorContains(t, err, "publish components output")
	assert.DirExists(t, componentsPath)
	assert.NoFileExists(t, publicationPath)
}

func TestWriteComposeOutputs_reports_publication_publish_failure_after_components_commit(t *testing.T) {
	// Given
	root := t.TempDir()
	publicationPath := filepath.Join(root, "publication.json")
	require.NoError(t, os.Mkdir(publicationPath, 0o755))
	componentsPath := filepath.Join(root, "components.json")
	result := &ComposeResult{PublicationJSON: []byte("publication"), ComponentsJSON: []byte("components")}

	// When
	err := WriteComposeOutputs(publicationPath, componentsPath, result)

	// Then
	require.ErrorContains(t, err, "publish publication output")
	assert.DirExists(t, publicationPath)
	assert.Equal(t, result.ComponentsJSON, readComposeOutput(t, componentsPath))
}

func readComposeOutput(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return data
}
