package publication

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWriteComposeOutputs_replaces_both_outputs_with_private_exact_bytes(t *testing.T) {
	// Given
	root := t.TempDir()
	publicationPath := filepath.Join(root, "publication.json")
	componentsPath := filepath.Join(root, "components.json")
	require.NoError(t, os.WriteFile(publicationPath, []byte("old publication"), 0o644))
	require.NoError(t, os.WriteFile(componentsPath, []byte("old components"), 0o644))
	result := ComposeResult{PublicationJSON: []byte(`{"publication":"sentinel"}`), ComponentsJSON: []byte(`[{"component":"sentinel"}]`)}

	// When
	err := WriteComposeOutputs(publicationPath, componentsPath, &result)

	// Then
	require.NoError(t, err)
	publication, err := os.ReadFile(publicationPath)
	require.NoError(t, err)
	components, err := os.ReadFile(componentsPath)
	require.NoError(t, err)
	require.Equal(t, result.PublicationJSON, publication)
	require.Equal(t, result.ComponentsJSON, components)
	publicationInfo, err := os.Stat(publicationPath)
	require.NoError(t, err)
	componentsInfo, err := os.Stat(componentsPath)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), publicationInfo.Mode().Perm())
	require.Equal(t, os.FileMode(0o600), componentsInfo.Mode().Perm())
	require.Empty(t, composeOutputTemporaryFiles(t, root))
}

func TestWriteComposeOutputs_rejects_missing_or_aliased_output_contracts(t *testing.T) {
	// Given
	result := &ComposeResult{}
	tests := []struct {
		name            string
		publicationPath string
		componentsPath  string
		result          *ComposeResult
	}{
		{name: "nil result", publicationPath: "publication.json", componentsPath: "components.json"},
		{name: "missing publication path", componentsPath: "components.json", result: result},
		{name: "missing components path", publicationPath: "publication.json", result: result},
		{name: "aliased paths", publicationPath: "same.json", componentsPath: "same.json", result: result},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// When
			err := WriteComposeOutputs(test.publicationPath, test.componentsPath, test.result)

			// Then
			require.ErrorIs(t, err, ErrComposeInvalid)
		})
	}
}

func TestWriteComposeOutputs_cleans_staged_files_when_publication_staging_fails(t *testing.T) {
	// Given
	root := t.TempDir()
	componentsPath := filepath.Join(root, "components.json")
	publicationPath := filepath.Join(root, "missing", "publication.json")
	result := ComposeResult{PublicationJSON: []byte("publication"), ComponentsJSON: []byte("components")}

	// When
	err := WriteComposeOutputs(publicationPath, componentsPath, &result)

	// Then
	require.ErrorContains(t, err, "stage output")
	require.NoFileExists(t, componentsPath)
	require.Empty(t, composeOutputTemporaryFiles(t, root))
}

func TestWriteComposeOutputs_keeps_publication_unpublished_when_components_rename_fails(t *testing.T) {
	// Given
	root := t.TempDir()
	componentsPath := filepath.Join(root, "components")
	publicationPath := filepath.Join(root, "publication.json")
	require.NoError(t, os.Mkdir(componentsPath, 0o755))
	result := ComposeResult{PublicationJSON: []byte("publication"), ComponentsJSON: []byte("components")}

	// When
	err := WriteComposeOutputs(publicationPath, componentsPath, &result)

	// Then
	require.ErrorContains(t, err, "publish components output")
	require.NoFileExists(t, publicationPath)
	require.DirExists(t, componentsPath)
	require.Empty(t, composeOutputTemporaryFiles(t, root))
}

func TestWriteComposeOutputs_reports_partial_publish_when_publication_rename_fails(t *testing.T) {
	// Given
	root := t.TempDir()
	componentsPath := filepath.Join(root, "components.json")
	publicationPath := filepath.Join(root, "publication")
	require.NoError(t, os.Mkdir(publicationPath, 0o755))
	result := ComposeResult{PublicationJSON: []byte("publication"), ComponentsJSON: []byte("components")}

	// When
	err := WriteComposeOutputs(publicationPath, componentsPath, &result)

	// Then
	require.ErrorContains(t, err, "publish publication output")
	components, readErr := os.ReadFile(componentsPath)
	require.NoError(t, readErr)
	require.Equal(t, result.ComponentsJSON, components)
	require.DirExists(t, publicationPath)
	require.Empty(t, composeOutputTemporaryFiles(t, root))
}

func TestWriteComposeOutputs_fails_when_first_output_directory_is_missing(t *testing.T) {
	// Given
	root := t.TempDir()
	result := ComposeResult{PublicationJSON: []byte("publication"), ComponentsJSON: []byte("components")}

	// When
	err := WriteComposeOutputs(filepath.Join(root, "publication.json"), filepath.Join(root, "missing", "components.json"), &result)

	// Then
	require.ErrorContains(t, err, "stage output")
	require.Empty(t, composeOutputTemporaryFiles(t, root))
}

func composeOutputTemporaryFiles(t *testing.T, root string) []string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(root, ".publication-compose-*"))
	require.NoError(t, err)
	return paths
}
