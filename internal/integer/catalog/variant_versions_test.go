package catalog_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/integer/catalog"
)

const variantOnlyYAML = `
name: distroless/static
description: "Pure static runtime"
upstream:
  package: wolfi-baselayout
types:
  default:
    base: verity-static
versions:
  latest:
    latest: true
  nonroot: {}
  debug: {}
`

const latestPlusNumericYAML = `
name: mixed
description: "Mixed streams"
upstream:
  package: "mixed-{{version}}"
types:
  default:
    base: wolfi-base
versions:
  latest: {}
  "22": {}
`

func TestGenerate_VariantVersions_LatestTagStaysOnLatest(t *testing.T) {
	imagesDir := t.TempDir()
	writeFile(t, imagesDir, "static.yaml", variantOnlyYAML)

	cat, err := catalog.Generate(imagesDir, "", "ghcr.io/verity-org", nil, nil)
	require.NoError(t, err)

	require.Len(t, cat.Images, 1)
	img := cat.Images[0]
	for _, v := range img.Versions {
		require.Len(t, v.Variants, 1)
		assert.Equal(t, []string{v.Version}, v.Variants[0].Tags)
		assert.Equal(t, v.Version == "latest", v.Latest)
	}
}

func TestGenerate_NumericVersionBeatsExplicitLatest(t *testing.T) {
	imagesDir := t.TempDir()
	writeFile(t, imagesDir, "mixed.yaml", latestPlusNumericYAML)

	cat, err := catalog.Generate(imagesDir, "", "ghcr.io/verity-org", nil, nil)
	require.NoError(t, err)

	require.Len(t, cat.Images, 1)
	for _, v := range cat.Images[0].Versions {
		assert.Equal(t, v.Version == "22", v.Latest, "version %s", v.Version)
	}
}
