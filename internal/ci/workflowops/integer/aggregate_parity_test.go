package integer

import (
	"fmt"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/ci/workflowops/githubapi"
)

func TestAggregator_characterizes_report_mismatch_stages(t *testing.T) {
	tests := []struct {
		name      string
		reports   []string
		wantStage string
	}{
		{name: "stale batch", reports: []string{reportJSON("alpha", "1", "default", "success", "", "41-1", 1)}, wantStage: "batch-mismatch"},
		{name: "duplicate current report", reports: []string{reportJSON("alpha", "1", "default", "success", "", "42-1", 1), reportJSON("alpha", "1", "default", "success", "", "42-1", 1)}, wantStage: "duplicate-child-report"},
		{name: "wrong shard", reports: []string{reportJSON("alpha", "1", "default", "success", "", "42-1", 2)}, wantStage: "wrong-shard-report"},
		{name: "failed stage", reports: []string{reportJSON("alpha", "1", "default", "failure", "trivy", "42-1", 1)}, wantStage: "trivy"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given: one exact plan entry and the characterized child report set.
			tmp := t.TempDir()
			expected := filepath.Join(tmp, "expected.json")
			results := filepath.Join(tmp, "results")
			writeFixture(t, expected, `[{"name":"alpha","version":"1","type":"default"}]`)
			for index, report := range test.reports {
				writeFixture(t, filepath.Join(results, fmt.Sprintf("report-%d", index), "report.json"), report)
			}
			options := aggregateOptions(t, expected, results, "success")

			// When: the reports are correlated with the exact plan and batch.
			result, err := (Aggregator{}).Aggregate(t.Context(), &options)

			// Then: the same explicit failure stage is produced.
			require.ErrorIs(t, err, ErrAggregationFailed)
			require.NotEmpty(t, result.Failures)
			assert.Equal(t, test.wantStage, result.Failures[0].Stage)
		})
	}
}

func TestAggregator_accepts_complete_current_batch(t *testing.T) {
	// Given: every planned entry has exactly one successful report in the expected shard.
	tmp := t.TempDir()
	expected := filepath.Join(tmp, "expected.json")
	results := filepath.Join(tmp, "results")
	writeFixture(t, expected, `[{"name":"alpha","version":"1","type":"default"}]`)
	writeFixture(t, filepath.Join(results, "alpha", "report.json"), reportJSON("alpha", "1", "default", "success", "", "42-1", 1))

	// When: the complete batch is aggregated.
	options := aggregateOptions(t, expected, results, "success")
	result, err := (Aggregator{}).Aggregate(t.Context(), &options)

	// Then: success is reported with the shell-compatible shard summary.
	require.NoError(t, err)
	assert.Equal(t, "All 1 planned Integer child builds succeeded across 1 shard(s).", result.Message)
}

func TestAggregator_accepts_skipped_empty_plan(t *testing.T) {
	// Given: Integer discovery produced an empty plan.
	tmp := t.TempDir()
	expected := filepath.Join(tmp, "expected.json")
	results := filepath.Join(tmp, "results")
	writeFixture(t, expected, `[]`)
	childResult, err := ParseChildResult("skipped")
	require.NoError(t, err)
	repository, err := githubapi.NewRepository("verity-org/verity")
	require.NoError(t, err)

	// When: the skipped child matrix is aggregated.
	result, err := (Aggregator{}).Aggregate(t.Context(), &Options{
		ExpectedPath: expected, ResultsDir: results, ChildResult: childResult,
		Repository: repository, RunID: 42, BatchID: "42-1",
		SourceSHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	})

	// Then: the intentional no-op succeeds.
	require.NoError(t, err)
	assert.Equal(t, "No Integer child builds were dispatched.", result.Message)
}

func aggregateOptions(t *testing.T, expected, results, child string) Options {
	t.Helper()
	childResult, err := ParseChildResult(child)
	require.NoError(t, err)
	repository, err := githubapi.NewRepository("verity-org/verity")
	require.NoError(t, err)
	return Options{
		ExpectedPath: expected, ResultsDir: results, ChildResult: childResult,
		Repository: repository, RunID: 42, BatchID: "42-1",
		SourceSHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}
}

func reportJSON(image, version, variant, status, stage, batch string, shard int) string {
	return `{"image":"` + image + `","version":"` + version + `","type":"` + variant +
		`","status":"` + status + `","failure_stage":"` + stage +
		`","run_id":"42","run_attempt":1,"source_sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",` +
		`"repository":"verity-org/verity","batch_id":"` + batch + `","shard":` + strconv.Itoa(shard) + `}`
}
