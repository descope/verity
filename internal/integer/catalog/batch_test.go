package catalog_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/integer/catalog"
)

func TestGenerateWithOptions_rejects_stale_report_when_batch_mismatches(t *testing.T) {
	// Given: a successful report from an older nightly batch.
	imagesDir := t.TempDir()
	reportsDir := t.TempDir()
	writeFile(t, imagesDir, "node.yaml", nodeYAML)
	reportData, err := json.Marshal(map[string]any{
		"batch_id": "100-1",
		"digest":   "sha256:stale",
		"status":   "success",
		"built_at": "2026-01-01T00:00:00Z",
	})
	require.NoError(t, err)
	reportPath := filepath.Join(reportsDir, "node", "22", "default", "latest.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(reportPath), 0o755))
	require.NoError(t, os.WriteFile(reportPath, reportData, 0o644))

	// When: the catalog is generated for the current nightly batch.
	generated, err := catalog.GenerateWithOptions(catalog.Options{
		ImagesDir:       imagesDir,
		ReportsDir:      reportsDir,
		Registry:        "ghcr.io/verity-org",
		Packages:        testPkgs,
		ExpectedBatchID: "200-1",
	})
	require.NoError(t, err)

	// Then: stale success data is rejected instead of being published as current.
	variant := generated.Images[0].Versions[0].Variants[0]
	assert.Equal(t, "unknown", variant.Status)
	assert.Empty(t, variant.Digest)
	assert.Empty(t, variant.BuiltAt)
}
