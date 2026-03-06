package catalog_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/integer/apkindex"
	"github.com/verity-org/verity/internal/integer/catalog"
	"github.com/verity-org/verity/internal/integer/eol"
)

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

const nodeYAML = `
name: node
description: "Node.js runtime"
upstream:
  package: "nodejs-{{version}}"
types:
  default:
    base: wolfi-base
    packages: ["nodejs-{{version}}", "libstdc++"]
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

var testPkgs = []apkindex.Package{
	{Name: "nodejs-22"},
	{Name: "nodejs-24"},
}

func TestGenerate_NoReports(t *testing.T) {
	imagesDir := t.TempDir()
	writeFile(t, imagesDir, "node.yaml", nodeYAML)

	cat, err := catalog.Generate(imagesDir, "", "ghcr.io/verity-org", testPkgs, nil)
	require.NoError(t, err)

	require.Len(t, cat.Images, 1)
	img := cat.Images[0]
	assert.Equal(t, "node", img.Name)
	assert.Equal(t, "Node.js runtime", img.Description)
	require.Len(t, img.Versions, 2)

	v22 := img.Versions[0]
	assert.Equal(t, "22", v22.Version)
	assert.False(t, v22.Latest)
	require.Len(t, v22.Variants, 2)

	// Variants are sorted by type name: default < dev
	defVariant := v22.Variants[0]
	assert.Equal(t, "default", defVariant.Type)
	assert.Equal(t, []string{"22"}, defVariant.Tags)
	assert.Equal(t, "ghcr.io/verity-org/node:22", defVariant.Ref)
	assert.Equal(t, "unknown", defVariant.Status)
	assert.Empty(t, defVariant.Digest)

	devVariant := v22.Variants[1]
	assert.Equal(t, "dev", devVariant.Type)
	assert.Equal(t, []string{"22-dev"}, devVariant.Tags)

	v24 := img.Versions[1]
	assert.True(t, v24.Latest)
	require.Len(t, v24.Variants, 2)
	assert.Equal(t, []string{"24", "latest"}, v24.Variants[0].Tags)

	assert.Equal(t, "ghcr.io/verity-org", cat.Registry)
	assert.NotEmpty(t, cat.GeneratedAt)
}

func TestGenerate_WithReports(t *testing.T) {
	imagesDir := t.TempDir()
	reportsDir := t.TempDir()
	writeFile(t, imagesDir, "node.yaml", nodeYAML)

	report := map[string]any{
		"digest":   "sha256:abc123",
		"status":   "success",
		"built_at": "2026-01-01T00:00:00Z",
	}
	reportData, err := json.Marshal(report)
	require.NoError(t, err)
	reportPath := filepath.Join(reportsDir, "node", "22", "default", "latest.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(reportPath), 0o755))
	require.NoError(t, os.WriteFile(reportPath, reportData, 0o644))

	cat, err := catalog.Generate(imagesDir, reportsDir, "ghcr.io/verity-org", testPkgs, nil)
	require.NoError(t, err)

	v22 := cat.Images[0].Versions[0]
	defVariant := v22.Variants[0]
	assert.Equal(t, "success", defVariant.Status)
	assert.Equal(t, "sha256:abc123", defVariant.Digest)
	assert.Equal(t, "2026-01-01T00:00:00Z", defVariant.BuiltAt)

	// dev variant has no report → status stays "unknown"
	assert.Equal(t, "unknown", v22.Variants[1].Status)
}

func TestGenerate_SkipsNonYAML(t *testing.T) {
	imagesDir := t.TempDir()
	writeFile(t, imagesDir, "node.yaml", nodeYAML)
	writeFile(t, imagesDir, "README.md", "# readme")

	cat, err := catalog.Generate(imagesDir, "", "ghcr.io/verity-org", testPkgs, nil)
	require.NoError(t, err)
	assert.Len(t, cat.Images, 1)
}

func TestGenerate_InvalidImagesDir(t *testing.T) {
	_, err := catalog.Generate("/nonexistent/path", "", "ghcr.io/verity-org", nil, nil)
	require.Error(t, err)
}

func TestGenerate_EmptyImagesDir(t *testing.T) {
	imagesDir := t.TempDir()
	cat, err := catalog.Generate(imagesDir, "", "ghcr.io/verity-org", nil, nil)
	require.NoError(t, err)
	assert.Empty(t, cat.Images)
}

func TestGenerate_CorruptReport(t *testing.T) {
	imagesDir := t.TempDir()
	reportsDir := t.TempDir()
	writeFile(t, imagesDir, "node.yaml", nodeYAML)

	reportPath := filepath.Join(reportsDir, "node", "22", "default", "latest.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(reportPath), 0o755))
	require.NoError(t, os.WriteFile(reportPath, []byte("not json"), 0o644))

	// Should not fail — corrupt report is silently skipped; status stays "unknown"
	cat, err := catalog.Generate(imagesDir, reportsDir, "ghcr.io/verity-org", testPkgs, nil)
	require.NoError(t, err)
	assert.Equal(t, "unknown", cat.Images[0].Versions[0].Variants[0].Status)
}

func TestGenerate_MultipleImages(t *testing.T) {
	imagesDir := t.TempDir()
	const pythonYAML = `
name: python
description: "Python runtime"
upstream:
  package: "python-{{version}}"
types:
  default:
    base: wolfi-base
    packages: ["python-{{version}}"]
    entrypoint: /usr/bin/python3
versions:
  "3.12": {}
`
	writeFile(t, imagesDir, "node.yaml", nodeYAML)
	writeFile(t, imagesDir, "python.yaml", pythonYAML)

	pkgs := []apkindex.Package{
		{Name: "nodejs-22"},
		{Name: "nodejs-24"},
		{Name: "python-3.12"},
	}

	cat, err := catalog.Generate(imagesDir, "", "ghcr.io/verity-org", pkgs, nil)
	require.NoError(t, err)
	assert.Len(t, cat.Images, 2)

	// Images are sorted by name.
	assert.Equal(t, "node", cat.Images[0].Name)
	assert.Equal(t, "python", cat.Images[1].Name)
}

func TestGenerate_NonExistentReportsDirErrors(t *testing.T) {
	imagesDir := t.TempDir()
	writeFile(t, imagesDir, "node.yaml", nodeYAML)

	_, err := catalog.Generate(imagesDir, "/nonexistent/reports", "ghcr.io/verity-org", nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reports dir")
}

func TestGenerate_EOLField(t *testing.T) {
	imagesDir := t.TempDir()
	writeFile(t, imagesDir, "node.yaml", nodeYAML)

	cat, err := catalog.Generate(imagesDir, "", "ghcr.io/verity-org", testPkgs, nil)
	require.NoError(t, err)
	assert.Equal(t, "2027-04-30", cat.Images[0].Versions[0].EOL)
}

func TestGenerate_AutoDiscoveredVersion(t *testing.T) {
	imagesDir := t.TempDir()
	writeFile(t, imagesDir, "node.yaml", nodeYAML)

	pkgs := []apkindex.Package{
		{Name: "nodejs-22"},
		{Name: "nodejs-24"},
		{Name: "nodejs-26"},
	}

	cat, err := catalog.Generate(imagesDir, "", "ghcr.io/verity-org", pkgs, nil)
	require.NoError(t, err)

	require.Len(t, cat.Images[0].Versions, 3)

	v22 := cat.Images[0].Versions[0]
	v24 := cat.Images[0].Versions[1]
	v26 := cat.Images[0].Versions[2]

	assert.False(t, v22.Latest)
	assert.False(t, v24.Latest)
	assert.True(t, v26.Latest)
	assert.Equal(t, []string{"26", "latest"}, v26.Variants[0].Tags)
}

type stubEOLFetcher struct {
	data map[string]eol.EOLData
}

func (s *stubEOLFetcher) FetchForImage(imageName string) (eol.EOLData, error) {
	if data, ok := s.data[imageName]; ok {
		return data, nil
	}
	return eol.EOLData{}, nil
}

func TestGenerate_WithEOLFetcher(t *testing.T) {
	imagesDir := t.TempDir()

	const nodeNoEOLYAML = `
name: node
description: "Node.js runtime"
upstream:
  package: "nodejs-{{version}}"
types:
  default:
    base: wolfi-base
    packages: ["nodejs-{{version}}"]
    entrypoint: /usr/bin/node
versions:
  "22": {}
  "24":
    latest: true
`
	writeFile(t, imagesDir, "node.yaml", nodeNoEOLYAML)

	fetcher := &stubEOLFetcher{
		data: map[string]eol.EOLData{
			"node": {
				"22": "2027-04-30",
				"24": "2028-04-30",
			},
		},
	}

	cat, err := catalog.Generate(imagesDir, "", "ghcr.io/verity-org", testPkgs, fetcher)
	require.NoError(t, err)

	require.Len(t, cat.Images, 1)
	require.Len(t, cat.Images[0].Versions, 2)

	assert.Equal(t, "2027-04-30", cat.Images[0].Versions[0].EOL)
	assert.Equal(t, "2028-04-30", cat.Images[0].Versions[1].EOL)
}

func TestGenerate_EOLFetcherOverridesYAML(t *testing.T) {
	imagesDir := t.TempDir()
	writeFile(t, imagesDir, "node.yaml", nodeYAML)

	fetcher := &stubEOLFetcher{
		data: map[string]eol.EOLData{
			"node": {
				"22": "2099-12-31",
			},
		},
	}

	cat, err := catalog.Generate(imagesDir, "", "ghcr.io/verity-org", testPkgs, fetcher)
	require.NoError(t, err)

	assert.Equal(t, "2099-12-31", cat.Images[0].Versions[0].EOL)
	assert.Equal(t, "2028-04-30", cat.Images[0].Versions[1].EOL)
}

func TestGenerate_EOLFetcherFallsBackToYAML(t *testing.T) {
	imagesDir := t.TempDir()
	writeFile(t, imagesDir, "node.yaml", nodeYAML)

	fetcher := &stubEOLFetcher{
		data: map[string]eol.EOLData{},
	}

	cat, err := catalog.Generate(imagesDir, "", "ghcr.io/verity-org", testPkgs, fetcher)
	require.NoError(t, err)

	assert.Equal(t, "2027-04-30", cat.Images[0].Versions[0].EOL)
	assert.Equal(t, "2028-04-30", cat.Images[0].Versions[1].EOL)
}

func TestGenerate_LatestSkipsEOLVersions(t *testing.T) {
	imagesDir := t.TempDir()

	const eolYAML = `
name: node
description: "Node.js runtime"
upstream:
  package: "nodejs-{{version}}"
types:
  default:
    base: wolfi-base
    packages: ["nodejs-{{version}}"]
    entrypoint: /usr/bin/node
versions:
  "20":
    eol: "2028-04-30"
  "22":
    eol: "2029-04-30"
  "24":
    eol: "2020-01-01"
`
	writeFile(t, imagesDir, "node.yaml", eolYAML)

	pkgs := []apkindex.Package{
		{Name: "nodejs-20"},
		{Name: "nodejs-22"},
		{Name: "nodejs-24"},
	}

	cat, err := catalog.Generate(imagesDir, "", "ghcr.io/verity-org", pkgs, nil)
	require.NoError(t, err)

	require.Len(t, cat.Images[0].Versions, 3)

	v20 := cat.Images[0].Versions[0]
	v22 := cat.Images[0].Versions[1]
	v24 := cat.Images[0].Versions[2]

	assert.False(t, v20.Latest)
	assert.True(t, v22.Latest, "v22 should be latest (v24 is EOL)")
	assert.False(t, v24.Latest, "v24 is EOL so should not be latest")
	assert.Equal(t, []string{"22", "latest"}, v22.Variants[0].Tags)
}

func TestGenerate_LatestIgnoresYAMLFlag(t *testing.T) {
	imagesDir := t.TempDir()

	const prometheusLike = `
name: prometheus
description: "Prometheus"
upstream:
  package: "prometheus-{{version}}"
types:
  default:
    base: wolfi-base
    packages: ["prometheus-{{version}}"]
    entrypoint: /usr/bin/prometheus
versions:
  "2.55": {}
  "3.9":
    latest: true
`
	writeFile(t, imagesDir, "prometheus.yaml", prometheusLike)

	pkgs := []apkindex.Package{
		{Name: "prometheus-2.55"},
		{Name: "prometheus-3.9"},
		{Name: "prometheus-3.10"},
	}

	cat, err := catalog.Generate(imagesDir, "", "ghcr.io/verity-org", pkgs, nil)
	require.NoError(t, err)

	require.Len(t, cat.Images[0].Versions, 3)

	for _, v := range cat.Images[0].Versions {
		if v.Version == "3.10" {
			assert.True(t, v.Latest, "3.10 should be latest (highest non-EOL)")
		} else {
			assert.False(t, v.Latest, "%s should not be latest", v.Version)
		}
	}
}

func TestGenerate_UnversionedPackageNoDuplicateTags(t *testing.T) {
	imagesDir := t.TempDir()

	const curlYAML = `
name: curl
description: "curl"
upstream:
  package: curl
types:
  default:
    base: wolfi-base
    packages: [curl]
    entrypoint: /usr/bin/curl
versions:
  "latest": {}
`
	writeFile(t, imagesDir, "curl.yaml", curlYAML)

	pkgs := []apkindex.Package{{Name: "curl"}}

	cat, err := catalog.Generate(imagesDir, "", "ghcr.io/verity-org", pkgs, nil)
	require.NoError(t, err)

	require.Len(t, cat.Images[0].Versions, 1)
	v := cat.Images[0].Versions[0]
	assert.True(t, v.Latest)
	assert.Equal(t, []string{"latest"}, v.Variants[0].Tags)
}
