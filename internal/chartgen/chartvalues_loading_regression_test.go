package chartgen

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"gopkg.in/yaml.v3"

	cfgpkg "github.com/verity-org/verity/internal/config"
	"github.com/verity-org/verity/internal/discovery"
)

// TestVerityConfigChartValuesLoading is a regression guard for the chart-gen
// strict-mode fix landed in PR #386 / #387: charts with verity.yaml
// chartValues entries (gitea) AND charts whose images are all in
// unpatchableImages (victoria-logs-single) must NOT be counted as
// strict-mode skips.
//
// This test ONLY validates the YAML loading path — full processChart
// coverage lives in processchart_emit_test.go.
func TestVerityConfigChartValuesLoading(t *testing.T) {
	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(repoRoot, "verity.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var vc cfgpkg.VerityConfig
	if err := yaml.Unmarshal(b, &vc); err != nil {
		t.Fatal(err)
	}
	if gv := vc.ChartValues["gitea"]; len(gv) == 0 {
		t.Fatalf("gitea chartValues missing — chartgen would treat it as zero-mappings skip")
	}

	vc2, err := discovery.LoadVerityConfig(filepath.Join(repoRoot, "verity.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if gv := vc2.ChartValues["gitea"]; len(gv) == 0 {
		t.Fatalf("LoadVerityConfig dropped gitea chartValues — chartgen would skip gitea")
	}

	// victoria-logs-single has NO chartValues but its only image is in
	// unpatchableImages — the new processChart passthrough branch must
	// handle it without a chartValues entry.
	if !slices.Contains(vc2.UnpatchableImages, "victoriametrics/victoria-logs") {
		t.Fatalf("expected victoriametrics/victoria-logs in unpatchableImages")
	}
}
