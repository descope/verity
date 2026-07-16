package ci

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlanIntegerPRStrictBuildVariantsAreExcludedFromSmokeCoverage(t *testing.T) {
	// Given: a changed family with two versions and two image types.
	root := setupIntegerPlanRepo(t)

	// When: the Integer PR matrices are planned.
	plan, err := PlanIntegerPR(&IntegerPROptions{
		ChangedFiles: []string{"images/node.yaml"},
		ConfigPath:   filepath.Join(root, "integer.yaml"),
		ImagesDir:    filepath.Join(root, "images"),
		GenDir:       filepath.Join(root, "gen"),
	})

	// Then: strict builds cover the latest variants and smoke retains only
	// the older family variants, with no exact overlap or lost coverage.
	require.NoError(t, err)
	require.True(t, plan.HasChanges)
	strict := []map[string]string{
		{"image": "node", "version": "22", "type": "default"},
		{"image": "node", "version": "22", "type": "dev"},
	}
	smokeOnly := []map[string]string{
		{"image": "node", "version": "20", "type": "default"},
		{"image": "node", "version": "20", "type": "dev"},
	}
	assert.ElementsMatch(t, strict, plan.Matrix.Include)
	assert.ElementsMatch(t, smokeOnly, plan.SmokeMatrix.Include)
	for _, entry := range plan.Matrix.Include {
		assert.NotContains(t, plan.SmokeMatrix.Include, entry)
	}
	expectedCoverage := make([]map[string]string, 0, len(strict)+len(smokeOnly))
	expectedCoverage = append(expectedCoverage, strict...)
	expectedCoverage = append(expectedCoverage, smokeOnly...)
	actualCoverage := make([]map[string]string, 0, len(plan.Matrix.Include)+len(plan.SmokeMatrix.Include))
	actualCoverage = append(actualCoverage, plan.Matrix.Include...)
	actualCoverage = append(actualCoverage, plan.SmokeMatrix.Include...)
	assert.ElementsMatch(t, expectedCoverage, actualCoverage)
}
