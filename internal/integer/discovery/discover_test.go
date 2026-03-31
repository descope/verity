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
    packages: ["prometheus-{{version}}"]
    entrypoint: /usr/bin/prometheus
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
    packages: ["prometheus-{{version}}"]
    entrypoint: /usr/bin/prometheus
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
