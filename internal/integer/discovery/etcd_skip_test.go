package discovery_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/integer/apkindex"
	"github.com/verity-org/verity/internal/integer/discovery"
)

func TestDiscoverFromFiles_SkipsUnavailableEtcd35Builds(t *testing.T) {
	const etcdYAML = `
name: etcd
description: "etcd"
upstream:
  package: "etcd-{{version}}"
types:
  default:
    base: wolfi-base
    packages: ["etcd-{{version}}"]
    entrypoint: /usr/bin/etcd
  fips:
    base: wolfi-fips
    packages: ["etcd-{{version}}"]
    entrypoint: /usr/bin/etcd
versions:
  "3.5":
    skip-types: [default, fips]
  "3.6": {}
`
	imagesDir := setupImages(t, map[string]string{"etcd.yaml": etcdYAML})
	genDir := t.TempDir()

	pkgs := []apkindex.Package{
		{Name: "etcd-3.5"},
		{Name: "etcd-3.6"},
	}

	imgs, err := discovery.DiscoverFromFiles(opts(imagesDir, genDir, pkgs))
	require.NoError(t, err)
	require.Len(t, imgs, 2)

	for _, img := range imgs {
		assert.Equal(t, "3.6", img.Version)
	}
}
