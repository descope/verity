package integer

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAggregator_rejects_valid_report_with_spoofed_conflicting_extra(t *testing.T) {
	// Given: one exact successful report plus one same-identity spoofed report.
	tmp := t.TempDir()
	expected := filepath.Join(tmp, "expected.json")
	results := filepath.Join(tmp, "results")
	writeFixture(t, expected, `[{"name":"alpha","version":"1","type":"default"}]`)
	writeFixture(t, filepath.Join(results, "valid", "report.json"), reportJSON("alpha", "1", "default", "success", "", "42-1", 1))
	writeFixture(t, filepath.Join(results, "spoof", "report.json"), `{
  "image":"alpha","version":"1","type":"default","status":"failure","failure_stage":"trivy",
  "run_id":"999","run_attempt":7,"source_sha":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
  "repository":"attacker/fork","batch_id":"42-1","shard":1
}`)
	options := aggregateOptions(t, expected, results, "success")

	// When: the complete report set is aggregated.
	result, err := (Aggregator{}).Aggregate(t.Context(), &options)

	// Then: the hostile extra is accounted for and blocks success.
	require.ErrorIs(t, err, ErrAggregationFailed)
	require.Len(t, result.Failures, 1)
	assert.Equal(t, "run-mismatch", result.Failures[0].Stage)
}

func TestAggregator_rejects_empty_plan_with_any_report(t *testing.T) {
	// Given: an empty plan accompanied by an undeclared exact-batch report.
	tmp := t.TempDir()
	expected := filepath.Join(tmp, "expected.json")
	results := filepath.Join(tmp, "results")
	writeFixture(t, expected, `[]`)
	writeFixture(t, filepath.Join(results, "beta", "report.json"), reportJSON("beta", "2", "default", "success", "", "42-1", 1))
	options := aggregateOptions(t, expected, results, "skipped")

	// When: the skipped no-op is aggregated.
	result, err := (Aggregator{}).Aggregate(t.Context(), &options)

	// Then: no-op success requires a truly empty report set.
	require.ErrorIs(t, err, ErrAggregationFailed)
	require.Len(t, result.Failures, 1)
	assert.Equal(t, "unexpected-child-report", result.Failures[0].Stage)
}

func TestAggregator_rejects_valid_report_with_mixed_stale_reports(t *testing.T) {
	tests := []struct {
		name      string
		stale     string
		wantStage string
	}{
		{
			name:      "same identity",
			stale:     reportJSON("alpha", "1", "default", "success", "", "41-1", 1),
			wantStage: "batch-mismatch",
		},
		{
			name:      "undeclared identity",
			stale:     reportJSON("beta", "2", "default", "success", "", "41-1", 1),
			wantStage: "unexpected-child-report",
		},
	}

	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given: one valid current report plus a stale report in the same artifact set.
			tmp := t.TempDir()
			expected := filepath.Join(tmp, "expected.json")
			results := filepath.Join(tmp, "results")
			writeFixture(t, expected, `[{"name":"alpha","version":"1","type":"default"}]`)
			writeFixture(t, filepath.Join(results, "valid", "report.json"), reportJSON("alpha", "1", "default", "success", "", "42-1", 1))
			writeFixture(t, filepath.Join(results, fmt.Sprintf("stale-%d", index), "report.json"), test.stale)
			options := aggregateOptions(t, expected, results, "success")

			// When: all discovered reports are aggregated.
			result, err := (Aggregator{}).Aggregate(t.Context(), &options)

			// Then: stale extras are never filtered away beside a valid report.
			require.ErrorIs(t, err, ErrAggregationFailed)
			require.Len(t, result.Failures, 1)
			assert.Equal(t, test.wantStage, result.Failures[0].Stage)
		})
	}
}
