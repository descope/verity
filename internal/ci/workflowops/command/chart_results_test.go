package command

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAggregateChartResultsCommand_emits_only_exact_small_outputs(t *testing.T) {
	// Given exact identity environment and a GitHub output file.
	output := filepath.Join(t.TempDir(), "github-output")
	t.Setenv("GITHUB_EVENT_NAME", "workflow_call")
	t.Setenv("GITHUB_OUTPUT", output)
	t.Setenv("CHART_SOURCE_SHA", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	t.Setenv("CHART_RUN_ID", "42")
	t.Setenv("CHART_RUN_ATTEMPT", "3")
	t.Setenv("CHART_PUBLICATION_ID", "publication-42")
	t.Setenv("CHART_BATCH_ID", "batch-42")
	t.Setenv("CHART_ARTIFACT_NAME", "chart-publication-publication-42")
	t.Setenv("CHART_ARTIFACT_DIGEST", "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")

	// When the public workflowops command aggregates the declared results.
	err := New().Run(context.Background(), []string{
		"workflowops", "aggregate-chart-results",
		"--result", "discover-charts=success", "--result", "chart-test=success",
	})

	// Then exactly seven scalar outputs are emitted.
	require.NoError(t, err)
	data, readErr := os.ReadFile(output)
	require.NoError(t, readErr)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	assert.Len(t, lines, 7)
	assert.Contains(t, lines, "source_sha=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	assert.Contains(t, lines, "artifact_digest=sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
}

func TestAggregateChartResultsCommand_writes_no_outputs_when_results_fail(t *testing.T) {
	// Given a failed shard result and an absent output file.
	output := filepath.Join(t.TempDir(), "github-output")
	t.Setenv("GITHUB_EVENT_NAME", "pull_request")
	t.Setenv("GITHUB_OUTPUT", output)

	// When the public command receives the failed result set.
	err := New().Run(context.Background(), []string{
		"workflowops", "aggregate-chart-results",
		"--result", "discover-charts=success", "--result", "chart-test=failure",
	})

	// Then it fails closed before creating misleading outputs.
	require.Error(t, err)
	_, statErr := os.Stat(output)
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestAggregateChartResultsCommand_accepts_privileged_profile(t *testing.T) {
	// Given exact privileged producer identity and a GitHub output file.
	output := filepath.Join(t.TempDir(), "github-output")
	t.Setenv("GITHUB_OUTPUT", output)
	t.Setenv("CHART_SOURCE_SHA", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	t.Setenv("CHART_RUN_ID", "42")
	t.Setenv("CHART_RUN_ATTEMPT", "3")
	t.Setenv("CHART_PUBLICATION_ID", "publication-42")
	t.Setenv("CHART_BATCH_ID", "batch-42")
	t.Setenv("CHART_ARTIFACT_NAME", "chart-publication-publication-42")
	t.Setenv("CHART_ARTIFACT_DIGEST", "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")

	// When the public command validates the privileged result profile.
	err := New().Run(context.Background(), []string{
		"workflowops", "aggregate-chart-results", "--profile", "privileged",
		"--result", "chart-test=success",
	})

	// Then the exact small outputs are emitted.
	require.NoError(t, err)
	data, readErr := os.ReadFile(output)
	require.NoError(t, readErr)
	assert.Contains(t, string(data), "publication_id=publication-42\n")
}
