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
	assert.Empty(t, matrixOverlap(plan.Matrix.Include, plan.SmokeMatrix.Include))
	assert.ElementsMatch(t, matrixUnion(strict, smokeOnly), matrixUnion(plan.Matrix.Include, plan.SmokeMatrix.Include))
}

func matrixOverlap(left, right []map[string]string) []map[string]string {
	rightVariants := make(map[string]struct{}, len(right))
	for _, entry := range right {
		rightVariants[matrixVariantKey(entry)] = struct{}{}
	}
	overlap := make([]map[string]string, 0)
	for _, entry := range left {
		if _, ok := rightVariants[matrixVariantKey(entry)]; ok {
			overlap = append(overlap, entry)
		}
	}
	return overlap
}

func matrixUnion(left, right []map[string]string) []map[string]string {
	union := make([]map[string]string, 0, len(left)+len(right))
	union = append(union, left...)
	union = append(union, right...)
	return union
}

func matrixVariantKey(entry map[string]string) string {
	return entry["image"] + "\x00" + entry["version"] + "\x00" + entry["type"]
}
