package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/discovery"
	intdiscovery "github.com/verity-org/verity/internal/integer/discovery"
)

func TestNightlySourceTag(t *testing.T) {
	tests := map[string]string{
		"registry.k8s.io/foo/bar:v1.2.3": "v1.2.3",
		"docker.io/library/nginx":        "latest",
		"localhost:5000/ns/img:tag":      "tag",
		"repo/img@sha256:abc":            "",
	}
	for ref, want := range tests {
		t.Run(ref, func(t *testing.T) {
			assert.Equal(t, want, sourceTag(ref))
		})
	}
}

func TestNightlyTargetRefs(t *testing.T) {
	copa, err := copaTargetRef(discovery.DiscoveredImage{
		Name:           "prometheus/prometheus",
		Source:         "quay.io/prometheus/prometheus:v3.9.1",
		TargetRegistry: "ghcr.io/verity-org",
	})
	require.NoError(t, err)
	assert.Equal(t, "ghcr.io/verity-org/prometheus/prometheus:v3.9.1", copa)

	integer, err := integerTargetRef(intdiscovery.DiscoveredImage{
		Name:     "node",
		Version:  "24",
		Type:     "default",
		Tags:     []string{"24", "latest"},
		Registry: "ghcr.io/verity-org",
	})
	require.NoError(t, err)
	assert.Equal(t, "ghcr.io/verity-org/node:24", integer)

	integerRefs, err := integerTargetRefs(intdiscovery.DiscoveredImage{
		Name:     "node",
		Version:  "24",
		Type:     "dev",
		Tags:     []string{"24-dev", "24.18-dev", "latest-dev"},
		Registry: "ghcr.io/verity-org",
	})
	require.NoError(t, err)
	assert.Equal(t, []nightlyScanTarget{
		{ref: "ghcr.io/verity-org/node:24-dev", label: "ghcr.io/verity-org/node:24-dev"},
		{ref: "ghcr.io/verity-org/node:24.18-dev", label: "ghcr.io/verity-org/node:24.18-dev"},
		{ref: "ghcr.io/verity-org/node:latest-dev", label: "ghcr.io/verity-org/node:latest-dev"},
	}, integerRefs)
}

func TestNightlyDispatchInputs(t *testing.T) {
	dir := t.TempDir()

	copaPath := filepath.Join(dir, "copa.json")
	copaData, err := json.Marshal([]discovery.DiscoveredImage{{
		Name:           "library/nginx",
		Source:         "mirror.gcr.io/library/nginx:1.29.3",
		TargetRegistry: "ghcr.io/verity-org",
		Platforms:      "linux/amd64,linux/arm64",
		GoVcsURL:       "https://github.com/nginx/nginx@release-1.29.3",
	}})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(copaPath, copaData, 0o644))

	copa, err := nightlyDispatchInputs(nightlyFamilyCopa, copaPath)
	require.NoError(t, err)
	require.Equal(t, []map[string]string{{
		"image-name":      "library/nginx",
		"source-ref":      "mirror.gcr.io/library/nginx:1.29.3",
		"target-registry": "ghcr.io/verity-org",
		"platforms":       "linux/amd64,linux/arm64",
		"go-vcs-url":      "https://github.com/nginx/nginx@release-1.29.3",
	}}, copa)

	integerPath := filepath.Join(dir, "integer.json")
	integerData, err := json.Marshal([]intdiscovery.DiscoveredImage{{
		Name:     "node",
		Version:  "24",
		Type:     "dev",
		Tags:     []string{"24-dev", "latest-dev"},
		Registry: "ghcr.io/verity-org",
	}})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(integerPath, integerData, 0o644))

	integer, err := nightlyDispatchInputs(nightlyFamilyInteger, integerPath)
	require.NoError(t, err)
	require.Equal(t, []map[string]string{{
		"image":    "node",
		"version":  "24",
		"type":     "dev",
		"tags":     "24-dev,latest-dev",
		"registry": "ghcr.io/verity-org",
	}}, integer)
}
