package ci

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/config"
	copadiscovery "github.com/verity-org/verity/internal/discovery"
)

func TestPlanMatching_selects_known_value_replacement_and_fuzzy_charts(t *testing.T) {
	// Given
	charts := []config.ChartSpec{{Name: "alpha"}, {Name: "beta"}}
	chartNames := chartNameSet(charts)
	selected := map[string]struct{}{}
	valuesDir := filepath.Join(t.TempDir(), "values")
	writeTestFile(t, filepath.Join(valuesDir, "nested", "ignored.yaml"), "image: widget\n")
	writeTestFile(t, filepath.Join(valuesDir, "notes.txt"), "widget\n")
	writeTestFile(t, filepath.Join(valuesDir, "unknown.yaml"), "image: widget\n")
	writeTestFile(t, filepath.Join(valuesDir, "alpha.yaml"), "image: ghcr.io/verity-org/widget:1\n")

	// When
	addValueFileMatches(selected, filepath.Join(valuesDir, "missing"), "widget", chartNames)
	addValueFileMatches(selected, valuesDir, "widget", chartNames)
	addFuzzyChartMatches(selected, "beta", charts, []string{"beta", "missing"}, chartNames)
	addReplacementMatches(selected, "alpha", map[string]config.Replacement{"sentinel": {Image: "ghcr.io/verity-org/alpha"}}, charts, nil, chartNames)

	// Then
	require.Equal(t, []string{"alpha", "beta"}, sortedChartNames(selected))
}

func TestFirstCopaTagMatrix_selects_first_tag_per_image_stably(t *testing.T) {
	// Given
	images := []copadiscovery.DiscoveredImage{
		{Name: "beta", Source: "registry.invalid/beta"},
		{Name: "alpha", Source: "registry.invalid/alpha:2"},
		{Name: "alpha", Source: "registry.invalid/alpha:1"},
	}

	// When
	matrix := firstCopaTagMatrix(images)

	// Then
	require.Equal(t, []map[string]string{{"name": "alpha", "tag": "1"}, {"name": "beta", "tag": "latest"}}, matrix.Include)
}
