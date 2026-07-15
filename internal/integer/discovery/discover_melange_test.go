package discovery_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/integer/apkindex"
	"github.com/verity-org/verity/internal/integer/discovery"
)

func TestDiscoverFromFiles_VersionScopedBespokeOverridesOnlyMatchingTags(t *testing.T) {
	// Given: only stream 1.26 uses a bespoke recipe with a newer full version.
	const imageYAML = `
name: scoped-istio
description: scoped istio
upstream:
  package: "scoped-istio-{{version}}"
types:
  default:
    base: wolfi-base
    packages: ["scoped-istio-{{version}}"]
versions:
  "1.26":
    melange:
      default:
        bespoke: scoped-istio-1.26.yaml
  "1.30":
    latest: true
`
	const bespoke = `
package:
  name: scoped-istio-1.26
  version: "1.26.8"
  epoch: 0
`
	imagesDir := setupImages(t, map[string]string{"scoped-istio.yaml": imageYAML})
	writeFile(t, filepath.Dir(imagesDir), filepath.Join("packages", "bespoke", "scoped-istio-1.26.yaml"), bespoke)
	genDir := t.TempDir()
	pkgs := []apkindex.Package{
		{Name: "scoped-istio-1.26", Version: "1.26.4-r1"},
		{Name: "scoped-istio-1.30", Version: "1.30.1-r0"},
	}

	// When: image variants and semantic tags are discovered.
	imgs, err := discovery.DiscoverFromFiles(opts(imagesDir, genDir, pkgs))

	// Then: the bespoke version affects 1.26 while 1.30 keeps APKINDEX resolution.
	require.NoError(t, err)
	got := map[string][]string{}
	for _, img := range imgs {
		if img.Type == typeDefault {
			got[img.Version] = img.Tags
		}
	}
	assert.Equal(t, []string{"1.26", "1.26.8"}, got["1.26"])
	assert.Equal(t, []string{"1.30", "1.30.1", "latest"}, got["1.30"])
}
