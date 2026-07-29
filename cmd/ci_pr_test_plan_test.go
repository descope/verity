package cmd

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/ci"
)

func TestClassifyPRTestPaths_selects_Integer_and_Copa_scopes(t *testing.T) {
	tests := []struct {
		name    string
		paths   []string
		integer bool
		copa    bool
	}{
		{name: "documentation only", paths: []string{"docs/guide.md"}},
		{name: "Integer definition", paths: []string{"images/pushgateway.yaml"}, integer: true},
		{name: "Copa patch logic", paths: []string{"internal/patch/patch.go"}, copa: true},
		{name: "shared CI command", paths: []string{"cmd/ci_pr_test.go"}, integer: true, copa: true},
		{name: "PR workflow", paths: []string{".github/workflows/pr-test.yaml"}, integer: true, copa: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// When: changed paths cross the typed scope boundary.
			got := classifyPRTestPaths(test.paths)

			// Then: the same Integer and Copa suites remain selected.
			require.Equal(t, test.integer, got.Integer)
			require.Equal(t, test.copa, got.Copa)
		})
	}
}

func TestNewPRIntegerBatchMatrix_preserves_both_native_architectures(t *testing.T) {
	// Given: seventeen strict Integer entries, which cross the sixteen-entry batch boundary.
	include := make([]map[string]string, 17)
	for index := range include {
		include[index] = map[string]string{
			"image":   "image",
			"version": string(rune('a' + index)),
			"type":    "default",
		}
	}

	// When: the PR matrix is expanded into native runner batches.
	matrix, err := newPRIntegerBatchMatrix(ci.Matrix{Include: include})

	// Then: each source batch gets amd64 and arm64 legs with matching package architectures.
	require.NoError(t, err)
	require.Len(t, matrix.Include, 4)
	require.Equal(t, 0, matrix.Include[0].BatchID)
	require.Equal(t, "amd64", matrix.Include[0].Architecture)
	require.Equal(t, "x86_64", matrix.Include[0].PackageArchitecture)
	require.Len(t, matrix.Include[0].Entries, 16)
	require.Equal(t, "arm64", matrix.Include[1].Architecture)
	require.Equal(t, "aarch64", matrix.Include[1].PackageArchitecture)
	require.Equal(t, 1, matrix.Include[2].BatchID)
	require.Len(t, matrix.Include[2].Entries, 1)
}
