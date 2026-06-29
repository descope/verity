//go:build integration

package integration

import (
	"path/filepath"
	"testing"
)

func TestProductionSKIPSYAMLSkipsTempoDistributed(t *testing.T) {
	root, err := findRepoRoot()
	if err != nil {
		t.Skipf("findRepoRoot failed (likely not running in repo): %v", err)
	}
	path := filepath.Join(root, "test", "chart-integration", "SKIPS.yaml")
	cfg, err := LoadSkips(path)
	if err != nil {
		t.Fatalf("production SKIPS.yaml failed validation: %v", err)
	}
	if skipped, _ := cfg.IsSkipped("tempo-distributed"); !skipped {
		t.Fatal("tempo-distributed must stay skipped until ghcr.io/verity-org/tempo accepts the chart 1.61.3 distributed config")
	}
}
