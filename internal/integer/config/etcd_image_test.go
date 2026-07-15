package config

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestEtcd36DefaultUsesApprovedBespokePackage(t *testing.T) {
	// Given: the etcd image definition, locked recipe, and provenance lock.
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok, "locating test file")
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
	def, err := LoadImage(filepath.Join(repoRoot, "images", "etcd.yaml"))
	require.NoError(t, err)
	recipeData, err := os.ReadFile(filepath.Join(repoRoot, "packages", "bespoke", "locked", "etcd-3.6.yaml"))
	require.NoError(t, err)
	lockData, err := os.ReadFile(filepath.Join(repoRoot, "packages", "upstream.lock.json"))
	require.NoError(t, err)

	var recipe struct {
		Package struct {
			Name      string `yaml:"name"`
			Version   string `yaml:"version"`
			Epoch     int    `yaml:"epoch"`
			Copyright []struct {
				License string `yaml:"license"`
			} `yaml:"copyright"`
		} `yaml:"package"`
		Pipeline []struct {
			Name string `yaml:"name"`
			Uses string `yaml:"uses"`
			Runs string `yaml:"runs"`
			With struct {
				ExpectedCommit string `yaml:"expected-commit"`
				Deps           string `yaml:"deps"`
				ModRoot        string `yaml:"modroot"`
			} `yaml:"with"`
		} `yaml:"pipeline"`
	}
	require.NoError(t, yaml.Unmarshal(recipeData, &recipe))

	var lock struct {
		Packages map[string]struct {
			File        string `json:"file"`
			Version     string `json:"version"`
			SHA256      string `json:"sha256"`
			Source      string `json:"source"`
			CVETracking string `json:"cve_tracking"`
		} `json:"packages"`
	}
	require.NoError(t, json.Unmarshal(lockData, &lock))

	// When: the approved etcd 3.6 default matrix row is resolved.
	version36 := def.Versions["3.6"]
	defaultOverride := version36.Melange["default"]
	require.NotNil(t, defaultOverride)

	// Then: only that row selects the fixed, source-pinned Apache-2.0 package.
	require.Len(t, version36.Melange, 1)
	require.Equal(t, []string{"default", "fips"}, def.Versions["3.5"].SkipTypes)
	require.Equal(t, "etcd", defaultOverride.Upstream)
	require.Equal(t, "etcd-3.6", recipe.Package.Name)
	require.Equal(t, "3.6.13", recipe.Package.Version)
	require.Equal(t, 1, recipe.Package.Epoch)
	require.Len(t, recipe.Package.Copyright, 1)
	require.Equal(t, "Apache-2.0", recipe.Package.Copyright[0].License)

	var checkoutCommit, runtimeSmoke string
	bumpedModules := make(map[string]string)
	for _, step := range recipe.Pipeline {
		switch step.Uses {
		case "git-checkout":
			checkoutCommit = step.With.ExpectedCommit
		case "go/bump":
			bumpedModules[step.With.ModRoot] = step.With.Deps
		}
		if step.Name == "Smoke built etcd runtime" {
			runtimeSmoke = step.Runs
		}
	}
	require.Equal(t, "b0f9ef190952e6e66a778513097a02ee41220727", checkoutCommit)
	require.Len(t, bumpedModules, 4)
	for _, modRoot := range []string{"", "tests", "server", "etcdutl"} {
		require.Contains(t, bumpedModules[modRoot], "go.opentelemetry.io/otel/sdk@v1.44.0")
	}
	require.Contains(t, runtimeSmoke, `ETCD_BIN="${{targets.destdir}}/usr/bin/etcd"`)
	require.Contains(t, runtimeSmoke, `"$ETCD_BIN" --version`)
	require.Contains(t, runtimeSmoke, `endpoint health`)
	require.Contains(t, runtimeSmoke, `put verity-smoke "Hello, etcd"`)
	require.Contains(t, runtimeSmoke, `get verity-smoke --print-value-only`)

	locked := lock.Packages["etcd"]
	require.Equal(t, "etcd-3.6.yaml", locked.File)
	require.Equal(t, "3.6.13", locked.Version)
	require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(recipeData)), locked.SHA256)
	require.Equal(t, "public-recipe-baseline@3bc1a15922962c2ffbc0844b7f98672b79c5fd71", locked.Source)
	require.Contains(t, locked.CVETracking, "CVE-2026-41178")
	require.Contains(t, locked.CVETracking, "3.6.13-r1")
}
