package config_test

import (
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
