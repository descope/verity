package command

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChartMatrixCommand_writes_typed_matrix_outputs(t *testing.T) {
	// Given a reusable-run chart source and GitHub output file.
	repo := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(repo, "Chart.yaml"), []byte(
		"dependencies:\n  - name: alpha\n    version: 1.0.0\n    repository: https://example.invalid/alpha\n",
	), 0o600))
	output := filepath.Join(t.TempDir(), "github-output")
	t.Setenv("GITHUB_OUTPUT", output)

	// When the public workflow command builds the chart matrix.
	err := New().Run(context.Background(), []string{
		"workflowops", "chart-matrix", "--repo-root", repo, "--event-name", "workflow_call",
	})

	// Then compact matrix and strict outputs are emitted for GitHub Actions.
	require.NoError(t, err)
	data, readErr := os.ReadFile(output)
	require.NoError(t, readErr)
	assert.Equal(t, "matrix=[\"alpha\"]\nstrict=false\n", string(data))
}

func TestWriteChartSummaryCommand_appends_summary_file(t *testing.T) {
	// Given a GitHub step summary target.
	summaryPath := filepath.Join(t.TempDir(), "step-summary")
	t.Setenv("GITHUB_STEP_SUMMARY", summaryPath)

	// When the public workflow command records a privileged shard result.
	err := New().Run(context.Background(), []string{
		"workflowops", "write-chart-summary", "--chart", "cilium", "--outcome", "success", "--profile", "privileged",
	})

	// Then the typed summary is appended.
	require.NoError(t, err)
	data, readErr := os.ReadFile(summaryPath)
	require.NoError(t, readErr)
	assert.Equal(t, "## ✅ cilium privileged: success\n", string(data))
}
