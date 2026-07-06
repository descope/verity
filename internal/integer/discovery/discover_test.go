package discovery_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/integer/apkindex"
	"github.com/verity-org/verity/internal/integer/config"
	"github.com/verity-org/verity/internal/integer/discovery"
)

const (
	typeDefault = "default"
	typeDev     = "dev"
)

const nodeYAML = `
name: node
description: "Node.js"
upstream:
  package: "nodejs-{{version}}"
types:
  default:
    base: wolfi-base
    packages: ["nodejs-{{version}}"]
    entrypoint: /usr/bin/node
  dev:
    base: wolfi-dev
    packages: ["nodejs-{{version}}", "npm"]
    entrypoint: /usr/bin/node
versions:
  "22":
    eol: "2027-04-30"
  "24":
    eol: "2028-04-30"
    latest: true
`

// setupImages creates a minimal images/ + _base/ layout in a temp directory.
func setupImages(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()

	// Create _base/ with minimal base files.
	for _, base := range []string{"wolfi-base", "wolfi-dev", "wolfi-fips"} {
		writeFile(t, dir, "_base/"+base+".yaml", "# base\n")
	}

	for name, content := range files {
		writeFile(t, dir, name, content)
	}
	return dir
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func opts(imagesDir, genDir string, pkgs []apkindex.Package) discovery.Options {
	return discovery.Options{
		ImagesDir: imagesDir,
		Registry:  "ghcr.io/verity-org",
		Packages:  pkgs,
		GenDir:    genDir,
	}
}

func TestDiscoverFromFiles_Basic(t *testing.T) {
	imagesDir := setupImages(t, map[string]string{"node.yaml": nodeYAML})
	genDir := t.TempDir()

	pkgs := []apkindex.Package{{Name: "nodejs-22"}, {Name: "nodejs-24"}}

	imgs, err := discovery.DiscoverFromFiles(opts(imagesDir, genDir, pkgs))
	require.NoError(t, err)

	// 2 versions × 2 types = 4 images
	assert.Len(t, imgs, 4)

	for _, img := range imgs {
		assert.Equal(t, "ghcr.io/verity-org", img.Registry)
		assert.Equal(t, "node", img.Name)
	}

	for _, img := range imgs {
		switch {
		case img.Version == "24" && img.Type == typeDefault:
			assert.Equal(t, []string{"24", "latest"}, img.Tags)
		case img.Version == "22" && img.Type == typeDev:
			assert.Equal(t, []string{"22-dev"}, img.Tags)
		case img.Version == "24" && img.Type == typeDev:
			assert.Equal(t, []string{"24-dev", "latest-dev"}, img.Tags)
		case img.Version == "22" && img.Type == typeDefault:
			assert.Equal(t, []string{"22"}, img.Tags)
		}
	}
}

func TestDiscoverFromFiles_GeneratesApkoFiles(t *testing.T) {
	imagesDir := setupImages(t, map[string]string{"node.yaml": nodeYAML})
	genDir := t.TempDir()

	pkgs := []apkindex.Package{{Name: "nodejs-22"}, {Name: "nodejs-24"}}

	imgs, err := discovery.DiscoverFromFiles(opts(imagesDir, genDir, pkgs))
	require.NoError(t, err)

	for _, img := range imgs {
		assert.FileExists(t, img.File)
		data, err := os.ReadFile(img.File)
		require.NoError(t, err)
		// Each file should contain the package for its specific version.
		assert.Contains(t, string(data), "nodejs-"+img.Version, "file %s", img.File)
	}
}

func TestDiscoverFromFiles_NoAPKINDEX_UsesVersionsMap(t *testing.T) {
	imagesDir := setupImages(t, map[string]string{"node.yaml": nodeYAML})
	genDir := t.TempDir()

	// No packages — only versions map is used.
	imgs, err := discovery.DiscoverFromFiles(opts(imagesDir, genDir, nil))
	require.NoError(t, err)
	// 2 versions × 2 types = 4
	assert.Len(t, imgs, 4)
}

func TestDiscoverFromFiles_AutoDiscoverNewVersion(t *testing.T) {
	imagesDir := setupImages(t, map[string]string{"node.yaml": nodeYAML})
	genDir := t.TempDir()

	// APKINDEX has nodejs-26 which is NOT in the versions map.
	pkgs := []apkindex.Package{
		{Name: "nodejs-22"},
		{Name: "nodejs-24"},
		{Name: "nodejs-26"},
	}

	imgs, err := discovery.DiscoverFromFiles(opts(imagesDir, genDir, pkgs))
	require.NoError(t, err)
	// 3 versions × 2 types = 6
	assert.Len(t, imgs, 6)

	var v26 []discovery.DiscoveredImage
	for _, img := range imgs {
		if img.Version == "26" {
			v26 = append(v26, img)
		}
	}
	require.Len(t, v26, 2)
	for _, img := range v26 {
		if img.Type == typeDefault {
			assert.Equal(t, []string{"26", "latest"}, img.Tags)
		}
		if img.Type == typeDev {
			assert.Equal(t, []string{"26-dev", "latest-dev"}, img.Tags)
		}
	}

	// Version 24 no longer carries "latest" since 26 is higher.
	for _, img := range imgs {
		if img.Version == "24" && img.Type == typeDefault {
			assert.Equal(t, []string{"24"}, img.Tags)
		}
	}
}

func TestDiscoverFromFiles_SkipsNonYAML(t *testing.T) {
	imagesDir := setupImages(t, map[string]string{
		"node.yaml": nodeYAML,
		"README.md": "# readme",
		"notes.txt": "notes",
	})
	genDir := t.TempDir()

	imgs, err := discovery.DiscoverFromFiles(opts(imagesDir, genDir, nil))
	require.NoError(t, err)
	for _, img := range imgs {
		assert.Equal(t, "node", img.Name)
	}
}

func TestDiscoverFromFiles_InvalidYAML(t *testing.T) {
	imagesDir := setupImages(t, map[string]string{
		"broken.yaml": "not: valid: yaml: [",
	})
	genDir := t.TempDir()

	_, err := discovery.DiscoverFromFiles(opts(imagesDir, genDir, nil))
	require.Error(t, err)
}

func TestDiscoverFromFiles_EmptyDir(t *testing.T) {
	imagesDir := setupImages(t, nil)
	genDir := t.TempDir()

	imgs, err := discovery.DiscoverFromFiles(opts(imagesDir, genDir, nil))
	require.NoError(t, err)
	assert.Empty(t, imgs)
}

func TestDiscoverFromFiles_MultipleImages(t *testing.T) {
	const curlYAML = `
name: curl
upstream:
  package: curl
types:
  default:
    base: wolfi-base
    packages: [curl]
    entrypoint: /usr/bin/curl
versions:
  latest:
    latest: true
`
	imagesDir := setupImages(t, map[string]string{
		"node.yaml": nodeYAML,
		"curl.yaml": curlYAML,
	})
	genDir := t.TempDir()

	pkgs := []apkindex.Package{
		{Name: "nodejs-22"},
		{Name: "nodejs-24"},
		{Name: "curl"},
	}

	imgs, err := discovery.DiscoverFromFiles(opts(imagesDir, genDir, pkgs))
	require.NoError(t, err)

	names := make(map[string]bool)
	for _, img := range imgs {
		names[img.Name] = true
	}
	assert.True(t, names["node"])
	assert.True(t, names["curl"])
}

func TestShouldSkipType(t *testing.T) {
	def := &config.ImageDef{
		Types: map[string]config.TypeTemplate{
			"default": {},
			"fips":    {},
		},
		Versions: map[string]config.VersionMeta{
			"2.55": {SkipTypes: []string{"fips"}},
			"3.9":  {},
		},
	}

	assert.True(t, discovery.ShouldSkipType(def, "2.55", "fips"))
	assert.False(t, discovery.ShouldSkipType(def, "2.55", "default"))
	assert.False(t, discovery.ShouldSkipType(def, "3.9", "fips"))
	assert.False(t, discovery.ShouldSkipType(def, "3.9", "default"))
	assert.False(t, discovery.ShouldSkipType(def, "9.99", "fips"))
}

func TestDiscoverFromFiles_SkipTypes(t *testing.T) {
	const skipFipsYAML = `
name: prometheus
description: "Prometheus"
upstream:
  package: "prometheus-{{version}}"
types:
  default:
    base: wolfi-base
    packages: ["prometheus-{{version}}"]
    entrypoint: /usr/bin/prometheus
  fips:
    base: wolfi-base
    fips-profile: go
    packages: ["prometheus-{{version}}"]
    entrypoint: /usr/bin/prometheus
    environment:
      GODEBUG: "fips140=on"
    melange:
      upstream: "prometheus"
      env-file: "fips.env"
versions:
  "2.55":
    skip-types: [fips]
  "3.9": {}
`
	imagesDir := setupImages(t, map[string]string{"prometheus.yaml": skipFipsYAML})
	genDir := t.TempDir()

	pkgs := []apkindex.Package{
		{Name: "prometheus-2.55"},
		{Name: "prometheus-3.9"},
	}

	imgs, err := discovery.DiscoverFromFiles(opts(imagesDir, genDir, pkgs))
	require.NoError(t, err)

	// 2.55: default only (fips skipped) = 1
	// 3.9: default + fips = 2
	// Total = 3
	assert.Len(t, imgs, 3)

	for _, img := range imgs {
		if img.Version == "2.55" {
			assert.Equal(t, "default", img.Type, "2.55 should only have default, got %s", img.Type)
		}
	}

	var fipsCount int
	for _, img := range imgs {
		if img.Type == "fips" {
			fipsCount++
			assert.Equal(t, "3.9", img.Version)
		}
	}
	assert.Equal(t, 1, fipsCount)
}

func TestShouldSkipType_AutoDiscoveredMelange(t *testing.T) {
	def := &config.ImageDef{
		Types: map[string]config.TypeTemplate{
			"default": {},
			"fips": {
				Melange: &config.MelangeSpec{Upstream: "prometheus-3.9", EnvFile: "fips.env"},
			},
		},
		Versions: map[string]config.VersionMeta{
			"3.9": {},
		},
	}

	// Explicit version 3.9: fips is NOT skipped (no skip-types entry).
	assert.False(t, discovery.ShouldSkipType(def, "3.9", "fips"))
	assert.False(t, discovery.ShouldSkipType(def, "3.9", "default"))

	// Auto-discovered version 3.8 (not in Versions map):
	// fips IS skipped because it has a melange block.
	assert.True(t, discovery.ShouldSkipType(def, "3.8", "fips"))
	// default is NOT skipped (no melange block).
	assert.False(t, discovery.ShouldSkipType(def, "3.8", "default"))
}

func TestDiscoverFromFiles_AutoDiscoveredMelangeSkipped(t *testing.T) {
	const fipsImageYAML = `
name: prometheus
description: "Prometheus"
upstream:
  package: "prometheus-{{version}}"
types:
  default:
    base: wolfi-base
    packages: ["prometheus-{{version}}"]
    entrypoint: /usr/bin/prometheus
  fips:
    base: wolfi-base
    fips-profile: go
    packages: ["prometheus-{{version}}"]
    entrypoint: /usr/bin/prometheus
    environment:
      GODEBUG: "fips140=on"
    melange:
      upstream: "prometheus-3.9"
      env-file: "fips.env"
versions:
  "3.9": {}
`
	imagesDir := setupImages(t, map[string]string{"prometheus.yaml": fipsImageYAML})
	genDir := t.TempDir()

	// APKINDEX has 3.8 (auto-discovered) and 3.9 (explicit).
	pkgs := []apkindex.Package{
		{Name: "prometheus-3.8"},
		{Name: "prometheus-3.9"},
	}

	imgs, err := discovery.DiscoverFromFiles(opts(imagesDir, genDir, pkgs))
	require.NoError(t, err)

	// 3.8: default only (fips skipped because melange + auto-discovered) = 1
	// 3.9: default + fips = 2
	// Total = 3
	assert.Len(t, imgs, 3)

	for _, img := range imgs {
		if img.Version == "3.8" {
			assert.Equal(t, "default", img.Type, "auto-discovered 3.8 should only have default")
		}
	}

	var fipsVersions []string
	for _, img := range imgs {
		if img.Type == "fips" {
			fipsVersions = append(fipsVersions, img.Version)
		}
	}
	assert.Equal(t, []string{"3.9"}, fipsVersions, "only explicit version should get fips")
}

func TestDiscoverFromFiles_AutoDiscoveryDisabledUsesExplicitVersionsOnly(t *testing.T) {
	const teleportYAML = `
name: teleport
description: "Teleport"
upstream:
  package: "teleport-{{version}}"
  auto-discover: false
types:
  default:
    base: wolfi-base
    packages: ["teleport-{{version}}"]
    entrypoint: /usr/bin/teleport start
versions:
  "18.6": {}
`
	imagesDir := setupImages(t, map[string]string{"teleport.yaml": teleportYAML})
	genDir := t.TempDir()
	pkgs := []apkindex.Package{
		{Name: "teleport-17", Version: "17.7.8-r0"},
		{Name: "teleport-18", Version: "18.6.6-r0"},
		{Name: "teleport-18.6", Version: "18.6.6-r0"},
	}

	imgs, err := discovery.DiscoverFromFiles(opts(imagesDir, genDir, pkgs))
	require.NoError(t, err)
	require.Len(t, imgs, 1)
	assert.Equal(t, "18.6", imgs[0].Version)
	assert.Equal(t, typeDefault, imgs[0].Type)
}

func TestDiscoverFromFiles_ReviewFIPSProfileSkipped(t *testing.T) {
	const reviewYAML = `
name: review-only
upstream:
  package: review-only
types:
  default:
    base: wolfi-base
    packages: [review-only]
  fips:
    base: wolfi-base
    fips-profile: review
    packages: [review-only]
versions:
  latest:
    latest: true
`
	// Given: image with review-only FIPS variant.
	imagesDir := setupImages(t, map[string]string{"review-only.yaml": reviewYAML})
	genDir := t.TempDir()

	// When: images are discovered.
	imgs, err := discovery.DiscoverFromFiles(opts(imagesDir, genDir, []apkindex.Package{{Name: "review-only"}}))
	require.NoError(t, err)

	// Then: review FIPS variant is not discoverable/published.
	for _, img := range imgs {
		require.NotEqual(t, "fips", img.Type)
	}
}

func TestShouldSkipType_ReviewFIPSProfileSkipped(t *testing.T) {
	// Given: explicit version with review-only FIPS profile.
	def := &config.ImageDef{
		Types: map[string]config.TypeTemplate{
			"fips": {Base: "wolfi-base", FIPSProfile: config.FIPSProfileReview},
		},
		Versions: map[string]config.VersionMeta{"latest": {Latest: true}},
	}

	// When/Then: review variants are skipped even when explicitly declared.
	require.True(t, discovery.ShouldSkipType(def, "latest", "fips"))
}

func TestApplyTypeSuffix(t *testing.T) {
	imagesDir := setupImages(t, map[string]string{"node.yaml": nodeYAML})
	genDir := t.TempDir()

	pkgs := []apkindex.Package{{Name: "nodejs-22"}}
	imgs, err := discovery.DiscoverFromFiles(opts(imagesDir, genDir, pkgs))
	require.NoError(t, err)

	for _, img := range imgs {
		if img.Type == typeDefault {
			assert.NotContains(t, img.Tags[0], "-default")
		}
		if img.Type == typeDev {
			for _, tag := range img.Tags {
				assert.Contains(t, tag, "-dev")
			}
		}
	}
}

func TestSemverCascade(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{"three components", "22.16.0", []string{"22", "22.16", "22.16.0"}},
		{"dotted major.minor", "1.24.3", []string{"1", "1.24", "1.24.3"}},
		{"two components", "1.24", []string{"1", "1.24"}},
		{"single component", "22", []string{"22"}},
		{"empty", "", nil},
		{"four components", "1.2.3.4", []string{"1", "1.2", "1.2.3", "1.2.3.4"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := discovery.SemverCascade(tt.input)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestDeriveTags(t *testing.T) {
	tests := []struct {
		name          string
		streamVersion string
		latestVersion string
		fullVersion   string
		expected      []string
	}{
		{
			"major stream with full version",
			"22", "24", "22.16.0",
			[]string{"22", "22.16", "22.16.0"},
		},
		{
			"major stream latest with full version",
			"24", "24", "24.1.0",
			[]string{"24", "24.1", "24.1.0", "latest"},
		},
		{
			"dotted stream with full version",
			"1.24", "1.26", "1.24.3",
			[]string{"1.24", "1.24.3"},
		},
		{
			"no full version",
			"22", "24", "",
			[]string{"22"},
		},
		{
			"no full version latest",
			"24", "24", "",
			[]string{"24", "latest"},
		},
		{
			"latest stream with full version",
			"latest", "latest", "2.11.2",
			[]string{"latest", "2", "2.11", "2.11.2"},
		},
		{
			"latest stream no full version",
			"latest", "latest", "",
			[]string{"latest"},
		},
		{
			"stream equals full version",
			"22.16.0", "22.16.0", "22.16.0",
			[]string{"22.16.0", "latest"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := discovery.DeriveTags(tt.streamVersion, tt.latestVersion, tt.fullVersion)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestDiscoverFromFiles_FullSemverTags(t *testing.T) {
	imagesDir := setupImages(t, map[string]string{"node.yaml": nodeYAML})
	genDir := t.TempDir()

	// Packages WITH version fields — triggers semver expansion.
	pkgs := []apkindex.Package{
		{Name: "nodejs-22", Version: "22.16.0-r0"},
		{Name: "nodejs-24", Version: "24.1.0-r1"},
	}

	imgs, err := discovery.DiscoverFromFiles(opts(imagesDir, genDir, pkgs))
	require.NoError(t, err)
	assert.Len(t, imgs, 4)

	for _, img := range imgs {
		switch {
		case img.Version == "22" && img.Type == typeDefault:
			assert.Equal(t, []string{"22", "22.16", "22.16.0"}, img.Tags)
		case img.Version == "22" && img.Type == typeDev:
			assert.Equal(t, []string{"22-dev", "22.16-dev", "22.16.0-dev"}, img.Tags)
		case img.Version == "24" && img.Type == typeDefault:
			assert.Equal(t, []string{"24", "24.1", "24.1.0", "latest"}, img.Tags)
		case img.Version == "24" && img.Type == typeDev:
			assert.Equal(t, []string{"24-dev", "24.1-dev", "24.1.0-dev", "latest-dev"}, img.Tags)
		}
	}
}

func TestDiscoverFromFiles_BespokeVersionOverridesSemverTags(t *testing.T) {
	const istioYAML = `
name: istio-pilot
description: "Istio Pilot"
upstream:
  package: "istio-pilot-discovery-{{version}}"
types:
  default:
    base: wolfi-base
    packages: ["istio-pilot-discovery-{{version}}"]
    entrypoint: /usr/bin/pilot-discovery
    melange:
      bespoke: "istio-pilot-discovery-{{version}}.yaml"
versions:
  "1":
    latest: true
  "1.26": {}
  "1.30": {}
`
	const bespoke126 = `
package:
  name: istio-pilot-discovery-1.26
  version: "1.26.8"
  epoch: 4
`
	const bespoke1 = `
package:
  name: istio-pilot-discovery-1.30
  version: "1.30.2"
  epoch: 0
`
	const bespoke130 = `
package:
  name: istio-pilot-discovery-1.30
  version: "1.30.2"
  epoch: 0
`

	imagesDir := setupImages(t, map[string]string{"istio-pilot.yaml": istioYAML})
	writeFile(t, filepath.Dir(imagesDir), filepath.Join("packages", "bespoke", "istio-pilot-discovery-1.26.yaml"), bespoke126)
	writeFile(t, filepath.Dir(imagesDir), filepath.Join("packages", "bespoke", "istio-pilot-discovery-1.yaml"), bespoke1)
	writeFile(t, filepath.Dir(imagesDir), filepath.Join("packages", "bespoke", "istio-pilot-discovery-1.30.yaml"), bespoke130)
	genDir := t.TempDir()

	pkgs := []apkindex.Package{
		{Name: "istio-pilot-discovery-1.26", Version: "1.26.4-r1"},
		{Name: "istio-pilot-discovery-1.30", Version: "1.30.1-r0"},
	}

	imgs, err := discovery.DiscoverFromFiles(opts(imagesDir, genDir, pkgs))
	require.NoError(t, err)

	got := map[string][]string{}
	for _, img := range imgs {
		if img.Type == typeDefault {
			got[img.Version] = img.Tags
		}
	}

	assert.Equal(t, []string{"1", "1.30", "1.30.2"}, got["1"])
	assert.Equal(t, []string{"1.26", "1.26.8"}, got["1.26"])
	assert.Equal(t, []string{"1.30", "1.30.2", "latest"}, got["1.30"])
}

func TestDiscoverFromFiles_UnversionedLatestGuard(t *testing.T) {
	// Unversioned package (caddy-like) with explicit version stream "2".
	// ResolveVersions drops auto-discovered "latest" when explicit versions
	// exist, so only the "2" stream should be produced.
	const caddyYAML = `
name: caddy
description: "Caddy"
upstream:
  package: caddy
types:
  default:
    base: wolfi-base
    packages: [caddy]
    entrypoint: caddy
versions:
  "2": {}
`
	imagesDir := setupImages(t, map[string]string{"caddy.yaml": caddyYAML})
	genDir := t.TempDir()

	pkgs := []apkindex.Package{
		{Name: "caddy", Version: "2.11.2-r0"},
	}

	imgs, err := discovery.DiscoverFromFiles(opts(imagesDir, genDir, pkgs))
	require.NoError(t, err)

	// Only 1 version ("2") × 1 type ("default") = 1 image.
	require.Len(t, imgs, 1)
	assert.Equal(t, "2", imgs[0].Version)
	// Gets semver cascade + latest ("2" is the highest version).
	assert.Equal(t, []string{"2", "2.11", "2.11.2", "latest"}, imgs[0].Tags)
}

// TestDiscoverFromFiles_FloatingMajorAliasesToWolfiAPK is the regression
// guard for the chronic Integer Build Image bug
// (sample failing run: github.com/verity-org/verity/actions/runs/25254581240).
//
// Wolfi publishes "kyverno-1.17" but NOT a "kyverno-1" meta-package. Before
// this fix, an image config like
//
//	upstream:  { package: "kyverno-{{version}}" }
//	types.default.packages: ["kyverno-{{version}}"]
//	versions: { "1": {} }
//
// rendered an apko config containing the literal package "kyverno-1", which
// apko's apk solver could not satisfy at publish time
// (`failed to build image components: ... nothing provides "kyverno-1"`).
// 8-15 nightly dispatches failed this way every night, plus all the other
// floating-major streams listed in the bug report (cilium:1, crossplane:2,
// erlang:26/27/28, fluentd:1, prometheus:2.55, …).
//
// The renderer now aliases declared "1" → highest-matching-minor ("1.17")
// when the literal "kyverno-1" name is absent from the APKINDEX, so the
// rendered apko config contains "kyverno-1.17" — which apko CAN satisfy.
//
// The DiscoveredImage.Version stays "1" (so workflow dispatches by the
// declared stream still produce gen/kyverno/1/default.apko.yaml at the
// expected path); the rewrite happens via {{version}} substitution in
// render.Config and therefore applies to every templated field in the type
// — apko `packages:` constraints, env vars, entrypoint, paths, etc. The
// `packages:` constraint is the user-visible signal this test asserts on.
func TestDiscoverFromFiles_FloatingMajorAliasesToWolfiAPK(t *testing.T) {
	const kyvernoYAML = `
name: kyverno
description: "Kyverno"
upstream:
  package: "kyverno-{{version}}"
types:
  default:
    base: wolfi-base
    packages: ["kyverno-{{version}}"]
    entrypoint: /usr/bin/kyverno
versions:
  "1": {}
`
	imagesDir := setupImages(t, map[string]string{"kyverno.yaml": kyvernoYAML})
	genDir := t.TempDir()

	// Wolfi has only "kyverno-1.17" — no "kyverno-1" meta-package.
	// This is exactly the production state that triggers the bug.
	pkgs := []apkindex.Package{
		{Name: "kyverno-1.16", Version: "1.16.3-r0"},
		{Name: "kyverno-1.17", Version: "1.17.5-r0"},
	}

	imgs, err := discovery.DiscoverFromFiles(opts(imagesDir, genDir, pkgs))
	require.NoError(t, err)

	// Locate the declared "1" stream (auto-discovered "1.16", "1.17" stems
	// also appear in imgs because they're real Wolfi APKs — this fix
	// doesn't change auto-discovery behaviour, just floating-major rendering).
	var img *discovery.DiscoveredImage
	for i := range imgs {
		if imgs[i].Version == "1" {
			img = &imgs[i]
			break
		}
	}
	require.NotNil(t, img, "declared `1` stream missing from discovery output: %+v", imgs)

	// Workflow path contract: dispatch keyed by declared stream. The
	// integer-build-image.yaml workflow looks up
	// gen/${IMAGE}/${VERSION}/${TYPE}.apko.yaml — that path must still
	// be the declared "1" so dispatches don't break.
	assert.Contains(t, img.File, filepath.Join("kyverno", "1", "default.apko.yaml"))

	// Apko config contract: the rendered packages: list must reference a
	// real Wolfi APK ("kyverno-1.17"), NOT the unsatisfiable "kyverno-1"
	// that would surface as `nothing provides "kyverno-1"` at publish time.
	data, err := os.ReadFile(img.File)
	require.NoError(t, err)
	rendered := string(data)
	assert.Contains(t, rendered, "kyverno-1.17",
		"alias should resolve declared `1` to the highest minor Wolfi publishes")
	assert.NotContains(t, rendered, "- kyverno-1\n",
		"unaliased literal `kyverno-1` would crash apko publish — guard against regression")

	// Tag contract: tags still derive from the declared stream (so users
	// pulling ghcr.io/.../kyverno:1 keep working). Semver cascade picks up
	// the aliased full version ("1.17.5") for richer tags.
	assert.Contains(t, img.Tags, "1")
}

// TestDiscoverFromFiles_UnresolvableFloatingMajor verifies the renderer
// degrades gracefully when neither the literal name nor any minor exists
// in the APKINDEX. The discover step does NOT fail (apkindex outages must
// not bring down discovery); the rendered config will fail at apko publish
// with a clear `nothing provides …` error, and `verity integer validate
// --apkindex-url …` is the gate that catches this earlier at PR time.
func TestDiscoverFromFiles_UnresolvableFloatingMajor(t *testing.T) {
	const kyvernoYAML = `
name: kyverno
description: "Kyverno"
upstream:
  package: "kyverno-{{version}}"
types:
  default:
    base: wolfi-base
    packages: ["kyverno-{{version}}"]
    entrypoint: /usr/bin/kyverno
versions:
  "99": {}
`
	imagesDir := setupImages(t, map[string]string{"kyverno.yaml": kyvernoYAML})
	genDir := t.TempDir()

	// Wolfi knows nothing about kyverno-99 or any kyverno-99.X minor.
	pkgs := []apkindex.Package{
		{Name: "kyverno-1.17", Version: "1.17.5-r0"},
	}

	imgs, err := discovery.DiscoverFromFiles(opts(imagesDir, genDir, pkgs))
	require.NoError(t, err) // discovery must not fail; validate is the gate.

	var img *discovery.DiscoveredImage
	for i := range imgs {
		if imgs[i].Version == "99" {
			img = &imgs[i]
			break
		}
	}
	require.NotNil(t, img, "declared `99` stream missing from discovery output")

	data, err := os.ReadFile(img.File)
	require.NoError(t, err)
	// Unresolvable streams render the literal — preserves today's behaviour
	// for offline `verity integer discover` calls (Packages: nil) and lets
	// `integer validate --apkindex-url` flag this case explicitly at PR time.
	assert.Contains(t, string(data), "kyverno-99")
}

// TestDiscoverFromFiles_UnversionedUpstreamVersionedTypePackages is the
// regression for the erlang/haproxy/nginx shape flagged by review:
// `upstream.package` is unversioned ("erlang") but `types.<x>.packages`
// templates the version ("erlang-{{version}}"). Before the
// VersionedPackagePattern fix, `expandImage` passed `def.Upstream.Package`
// (= "erlang") to `ResolveAliasVersion`, which early-returned because the
// pattern had no `{{version}}` placeholder — leaving the rendered apko
// constraint as `erlang-26`, which Wolfi cannot satisfy. Locks in the
// behavior that alias resolution now uses the versioned pattern from the
// type's packages: list when upstream.package lacks the placeholder.
func TestDiscoverFromFiles_UnversionedUpstreamVersionedTypePackages(t *testing.T) {
	const erlangYAML = `
name: erlang
description: "Erlang/OTP"
upstream:
  package: erlang
types:
  default:
    base: wolfi-base
    packages: ["erlang-{{version}}"]
    entrypoint: /usr/bin/erl
versions:
  "26": {}
`
	imagesDir := setupImages(t, map[string]string{"erlang.yaml": erlangYAML})
	genDir := t.TempDir()

	// Wolfi has the meta-package "erlang" (so DiscoverVersions for the
	// upstream lookup succeeds) AND specific minor packages
	// "erlang-26.2" / "erlang-26.3" — but no "erlang-26" meta-package.
	// This is the production state that makes the Integer Build Image
	// runs for `erlang:26-default` fail with `nothing provides "erlang-26"`.
	pkgs := []apkindex.Package{
		{Name: "erlang", Version: "27.0-r0"}, // floating-latest
		{Name: "erlang-26.2", Version: "26.2.5.16-r0"},
		{Name: "erlang-26.3", Version: "26.3.0.0-r0"},
	}

	imgs, err := discovery.DiscoverFromFiles(opts(imagesDir, genDir, pkgs))
	require.NoError(t, err)

	var img *discovery.DiscoveredImage
	for i := range imgs {
		if imgs[i].Version == "26" {
			img = &imgs[i]
			break
		}
	}
	require.NotNil(t, img, "declared `26` stream missing from discovery output: %+v", imgs)

	// gen path keeps the declared stream so the workflow's
	// gen/${IMAGE}/${VERSION}/${TYPE}.apko.yaml lookup still resolves.
	assert.Contains(t, img.File, filepath.Join("erlang", "26", "default.apko.yaml"))

	// Apko config contract: the rendered packages: list must reference
	// the highest matching minor ("erlang-26.3"), NOT the unsatisfiable
	// "erlang-26" that the bug would render.
	data, err := os.ReadFile(img.File)
	require.NoError(t, err)
	rendered := string(data)
	assert.Contains(t, rendered, "erlang-26.3",
		"alias should resolve declared `26` to the highest minor Wolfi publishes (via type packages: pattern, since upstream.package is unversioned)")
	assert.NotContains(t, rendered, "- erlang-26\n",
		"unaliased literal `erlang-26` would crash apko publish — guard against regression")
}

// TestResolveStreamRenderVersion locks in the contract of the helper that
// the local CLI build path (cmd/integer_build.go) and the discovery path
// (expandImage) BOTH call to convert a declared stream version into the
// stem render.Config will substitute. Extracted so the two paths cannot
// drift again, the way they did between PR #307 (which fixed only
// discovery) and the follow-up PR that closed the build-path gap.
func TestResolveStreamRenderVersion(t *testing.T) {
	tests := []struct {
		name    string
		def     *config.ImageDef
		pkgs    []apkindex.Package
		stream  string
		want    string
		comment string
	}{
		{
			name: "literal match in upstream pattern → stream unchanged",
			def: &config.ImageDef{
				Upstream: config.Upstream{Package: "nodejs-{{version}}"},
				Types: map[string]config.TypeTemplate{
					"default": {Packages: []string{"nodejs-{{version}}"}},
				},
			},
			pkgs:   []apkindex.Package{{Name: "nodejs-22", Version: "22.0.0-r0"}},
			stream: "22",
			want:   "22",
		},
		{
			name: "floating-major aliased to minor (kyverno shape)",
			def: &config.ImageDef{
				Upstream: config.Upstream{Package: "kyverno-{{version}}"},
				Types: map[string]config.TypeTemplate{
					"default": {Packages: []string{"kyverno-{{version}}"}},
				},
			},
			pkgs:   []apkindex.Package{{Name: "kyverno-1.17", Version: "1.17.5-r0"}},
			stream: "1",
			want:   "1.17",
		},
		{
			name: "unversioned upstream + versioned type packages (erlang shape)",
			def: &config.ImageDef{
				Upstream: config.Upstream{Package: "erlang"},
				Types: map[string]config.TypeTemplate{
					"default": {Packages: []string{"erlang-{{version}}"}},
				},
			},
			pkgs: []apkindex.Package{
				{Name: "erlang", Version: "27.0-r0"},
				{Name: "erlang-26.3", Version: "26.3.0.0-r0"},
			},
			stream: "26",
			want:   "26.3",
		},
		{
			name: "no APKINDEX (offline) returns stream unchanged",
			def: &config.ImageDef{
				Upstream: config.Upstream{Package: "kyverno-{{version}}"},
				Types: map[string]config.TypeTemplate{
					"default": {Packages: []string{"kyverno-{{version}}"}},
				},
			},
			pkgs:   nil,
			stream: "1",
			want:   "1",
		},
		{
			name: "unsatisfiable stream (Wolfi dropped 2.x for prometheus) returns stream unchanged",
			def: &config.ImageDef{
				Upstream: config.Upstream{Package: "prometheus-{{version}}"},
				Types: map[string]config.TypeTemplate{
					"default": {Packages: []string{"prometheus-{{version}}"}},
				},
			},
			// APKINDEX has 3.x packages but no 2.55 / 2.55.x.
			pkgs: []apkindex.Package{
				{Name: "prometheus-3.9", Version: "3.9.1-r0"},
				{Name: "prometheus-3.11", Version: "3.11.3-r0"},
			},
			stream: "2.55",
			want:   "2.55",
		},
		{
			// Regression for nil-pointer dereference flagged on PR #311.
			// VersionedPackagePattern handles nil, but the fallback
			// `def.Upstream.Package` would panic. The exported helper
			// must no-op gracefully on nil — match the offline behavior.
			name:    "nil def returns stream unchanged (no panic)",
			def:     nil,
			pkgs:    []apkindex.Package{{Name: "kyverno-1.17", Version: "1.17.5-r0"}},
			stream:  "1",
			want:    "1",
			comment: "nil def must not panic; stream returns unchanged",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := discovery.ResolveStreamRenderVersion(tt.def, tt.pkgs, tt.stream)
			assert.Equal(t, tt.want, got, tt.comment)
		})
	}
}
