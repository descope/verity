package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/integer/config"
)

func TestImageFilePaths_includesNestedImagesAndSkipsBases(t *testing.T) {
	dir := t.TempDir()
	flat := writeFile(t, dir, "node.yaml", "name: node\n")
	nested := writeFile(t, dir, "distroless/static.yaml", "name: distroless/static\n")
	writeFile(t, dir, "_base/wolfi-base.yaml", "annotations: {}\n")

	paths, err := config.ImageFilePaths(dir)
	require.NoError(t, err)

	assert.Equal(t, []string{nested, flat}, paths)
	for _, path := range paths {
		assert.NotContains(t, filepath.ToSlash(path), "/_base/")
	}
}

func TestLoadImageByName_findsDirectDefinition(t *testing.T) {
	// Given: a definition at the conventional name-based path.
	dir := t.TempDir()
	writeFile(t, dir, "node.yaml", "name: node\n")

	// When: it is loaded by declared name.
	def, err := config.LoadImageByName(dir, "node")

	// Then: the direct path supplies the definition.
	require.NoError(t, err)
	assert.Equal(t, "node", def.Name)
}

func TestLoadImageByName_findsNestedDefinition(t *testing.T) {
	// Given: a nested definition whose file path and declared name differ.
	dir := t.TempDir()
	writeFile(t, dir, "platform/custom-file.yaml", "name: renamed-image\n")

	// When: it is loaded by declared name.
	def, err := config.LoadImageByName(dir, "renamed-image")

	// Then: recursive inventory lookup supplies the definition.
	require.NoError(t, err)
	assert.Equal(t, "renamed-image", def.Name)
}

func TestLoadImageByName_rejectsDuplicateDeclaredNames(t *testing.T) {
	// Given: two definitions with the same declared name.
	dir := t.TempDir()
	writeFile(t, dir, "duplicate.yaml", "name: duplicate\n")
	writeFile(t, dir, "nested/two.yaml", "name: duplicate\n")

	// When: the duplicate name is resolved.
	_, err := config.LoadImageByName(dir, "duplicate")

	// Then: lookup fails closed instead of choosing one path.
	require.ErrorIs(t, err, config.ErrDuplicateImageName)
}

func TestLoadImageByName_rejectsSymlinkedDefinition(t *testing.T) {
	// Given: a definition symlink that points outside imagesDir.
	root := t.TempDir()
	imagesDir := filepath.Join(root, "images")
	require.NoError(t, os.MkdirAll(imagesDir, 0o755))
	outside := writeFile(t, root, "outside.yaml", "name: escaped\n")
	require.NoError(t, os.Symlink(outside, filepath.Join(imagesDir, "escaped.yaml")))

	// When: the symlinked declared name is resolved.
	_, err := config.LoadImageByName(imagesDir, "escaped")

	// Then: the non-regular inventory entry is rejected.
	require.ErrorIs(t, err, config.ErrInvalidImageFile)
}

func TestLoadImageByName_confinesTraversalNameToImagesDirectory(t *testing.T) {
	// Given: an outside definition whose declared name contains traversal.
	root := t.TempDir()
	imagesDir := filepath.Join(root, "images")
	require.NoError(t, os.MkdirAll(imagesDir, 0o755))
	writeFile(t, root, "outside.yaml", "name: ../outside\n")

	// When: the traversal-shaped name is resolved.
	_, err := config.LoadImageByName(imagesDir, "../outside")

	// Then: lookup stays inside imagesDir and reports no matching definition.
	require.ErrorIs(t, err, config.ErrImageNotFound)
}

func TestLoadImageByName_propagatesMalformedDirectDefinition(t *testing.T) {
	// Given: a malformed definition at the conventional direct path.
	dir := t.TempDir()
	writeFile(t, dir, "node.yaml", "name: [\n")

	// When: it is loaded by name.
	_, err := config.LoadImageByName(dir, "node")

	// Then: the parse error is preserved rather than reported as not found.
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing image")
}

func TestLoadImageByName_reportsMissingImagesDirectory(t *testing.T) {
	// Given: an images directory that does not exist.
	imagesDir := filepath.Join(t.TempDir(), "missing")

	// When: an image is resolved.
	_, err := config.LoadImageByName(imagesDir, "node")

	// Then: the directory listing error is returned.
	require.Error(t, err)
	assert.Contains(t, err.Error(), "list image definitions")
}
