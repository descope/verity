package integer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/ci/workflowops/githubapi"
)

func TestAggregator_rejects_successful_partial_dispatch(t *testing.T) {
	// Given: two planned entries but only one successful current-batch report.
	tmp := t.TempDir()
	expected := filepath.Join(tmp, "expected.json")
	results := filepath.Join(tmp, "results")
	writeFixture(t, expected, `[
  {"name":"alpha","version":"1","type":"default"},
  {"name":"beta","version":"2","type":"default"}
]`)
	writeFixture(t, filepath.Join(results, "alpha", "report.json"), `{
  "image":"alpha","version":"1","type":"default","status":"success",
  "failure_stage":"","run_id":"42","run_attempt":1,
  "source_sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","repository":"verity-org/verity",
  "batch_id":"42-1","shard":1
}`)
	repository, err := githubapi.NewRepository("verity-org/verity")
	require.NoError(t, err)
	childResult, err := ParseChildResult("success")
	require.NoError(t, err)

	// When: the exact batch is aggregated.
	result, err := (Aggregator{}).Aggregate(t.Context(), &Options{
		ExpectedPath: expected,
		ResultsDir:   results,
		ChildResult:  childResult,
		Repository:   repository,
		RunID:        42,
		BatchID:      "42-1",
		SourceSHA:    "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	})

	// Then: the missing child is named instead of accepting misleading matrix success.
	require.ErrorIs(t, err, ErrAggregationFailed)
	require.Len(t, result.Failures, 1)
	assert.Equal(t, "beta", result.Failures[0].Image)
	assert.Equal(t, "matrix-dispatch-or-report", result.Failures[0].Stage)
}

func TestAggregator_rejects_tampered_child_report(t *testing.T) {
	// Given: a plan and a report containing two JSON documents.
	tmp := t.TempDir()
	expected := filepath.Join(tmp, "expected.json")
	results := filepath.Join(tmp, "results")
	writeFixture(t, expected, `[{"name":"alpha","version":"1","type":"default"}]`)
	writeFixture(t, filepath.Join(results, "alpha", "report.json"), "{}\n{}\n")
	repository, err := githubapi.NewRepository("verity-org/verity")
	require.NoError(t, err)
	childResult, err := ParseChildResult("failure")
	require.NoError(t, err)

	// When: untrusted artifacts are parsed.
	_, err = (Aggregator{}).Aggregate(t.Context(), &Options{
		ExpectedPath: expected, ResultsDir: results, ChildResult: childResult,
		Repository: repository, RunID: 42, BatchID: "42-1",
		SourceSHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	})

	// Then: malformed reports fail before identity matching.
	require.ErrorIs(t, err, ErrInvalidChildReport)
}

func TestAggregator_rejects_mismatched_run_attempt_source_and_repository(t *testing.T) {
	// Given: a success-looking report whose provenance does not match the requested batch.
	tmp := t.TempDir()
	expected := filepath.Join(tmp, "expected.json")
	results := filepath.Join(tmp, "results")
	writeFixture(t, expected, `[{"name":"alpha","version":"1","type":"default"}]`)
	writeFixture(t, filepath.Join(results, "alpha", "report.json"), `{
  "image":"alpha","version":"1","type":"default","status":"success","failure_stage":"",
  "run_id":"999","run_attempt":7,"source_sha":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
  "repository":"attacker/fork","batch_id":"42-1","shard":1
}`)
	repository, err := githubapi.NewRepository("verity-org/verity")
	require.NoError(t, err)
	childResult, err := ParseChildResult("success")
	require.NoError(t, err)

	// When: aggregation is bound to the exact run, attempt, source, repository, and batch.
	_, err = (Aggregator{}).Aggregate(t.Context(), &Options{
		ExpectedPath: expected, ResultsDir: results, ChildResult: childResult,
		Repository: repository, RunID: 42, BatchID: "42-1",
		SourceSHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	})

	// Then: misleading success metadata fails closed.
	require.ErrorIs(t, err, ErrAggregationFailed)
}

func TestAggregator_rejects_batch_that_does_not_identify_expected_run(t *testing.T) {
	// Given: a requested run ID and an unrelated batch ID.
	tmp := t.TempDir()
	expected := filepath.Join(tmp, "expected.json")
	writeFixture(t, expected, `[]`)
	repository, err := githubapi.NewRepository("verity-org/verity")
	require.NoError(t, err)
	childResult, err := ParseChildResult("skipped")
	require.NoError(t, err)

	// When: aggregation starts with conflicting expected identity.
	_, err = (Aggregator{}).Aggregate(t.Context(), &Options{
		ExpectedPath: expected, ResultsDir: filepath.Join(tmp, "results"), ChildResult: childResult,
		Repository: repository, RunID: 42, BatchID: "777-9",
		SourceSHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	})

	// Then: options fail before any report can be accepted.
	require.ErrorIs(t, err, ErrInvalidAggregation)
}

func TestAggregator_rejects_non_strict_Integer_JSON(t *testing.T) {
	tests := []struct {
		name   string
		report string
	}{
		{name: "duplicate conflicting key", report: `{"image":"alpha","version":"1","type":"default","status":"failure","status":"success","failure_stage":"trivy","run_id":"42","run_attempt":1,"source_sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","repository":"verity-org/verity","batch_id":"42-1","shard":1}`},
		{name: "case variant key", report: `{"image":"alpha","version":"1","type":"default","Status":"success","failure_stage":"","run_id":"42","run_attempt":1,"source_sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","repository":"verity-org/verity","batch_id":"42-1","shard":1}`},
		{name: "unknown key", report: `{"image":"alpha","version":"1","type":"default","status":"success","failure_stage":"","run_id":"42","run_attempt":1,"source_sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","repository":"verity-org/verity","batch_id":"42-1","shard":1,"trusted":true}`},
		{name: "trailing value", report: `{"image":"alpha","version":"1","type":"default","status":"success","failure_stage":"","run_id":"42","run_attempt":1,"source_sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","repository":"verity-org/verity","batch_id":"42-1","shard":1} {}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given: a valid plan and hostile report syntax at the artifact boundary.
			tmp := t.TempDir()
			expected := filepath.Join(tmp, "expected.json")
			results := filepath.Join(tmp, "results")
			writeFixture(t, expected, `[{"name":"alpha","version":"1","type":"default"}]`)
			writeFixture(t, filepath.Join(results, "alpha", "report.json"), test.report)
			repository, err := githubapi.NewRepository("verity-org/verity")
			require.NoError(t, err)
			childResult, err := ParseChildResult("success")
			require.NoError(t, err)

			// When: the report is decoded.
			_, err = (Aggregator{}).Aggregate(t.Context(), &Options{
				ExpectedPath: expected, ResultsDir: results, ChildResult: childResult,
				Repository: repository, RunID: 42, BatchID: "42-1",
				SourceSHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			})

			// Then: duplicate, case-variant, unknown, and trailing data are rejected.
			require.ErrorIs(t, err, ErrInvalidChildReport)
		})
	}
}

func TestAggregator_rejects_non_strict_Integer_plan_JSON(t *testing.T) {
	tests := []struct {
		name string
		plan string
	}{
		{name: "duplicate key", plan: `[{"name":"spoof","name":"alpha","version":"1","type":"default"}]`},
		{name: "case variant key", plan: `[{"Name":"alpha","version":"1","type":"default"}]`},
		{name: "unknown key", plan: `[{"name":"alpha","version":"1","type":"default","trusted":true}]`},
		{name: "trailing value", plan: `[{"name":"alpha","version":"1","type":"default"}] {}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given: hostile syntax in the expected Integer plan.
			tmp := t.TempDir()
			expected := filepath.Join(tmp, "expected.json")
			writeFixture(t, expected, test.plan)
			repository, err := githubapi.NewRepository("verity-org/verity")
			require.NoError(t, err)
			childResult, err := ParseChildResult("success")
			require.NoError(t, err)

			// When: plan decoding starts.
			_, err = (Aggregator{}).Aggregate(t.Context(), &Options{
				ExpectedPath: expected, ResultsDir: filepath.Join(tmp, "results"), ChildResult: childResult,
				Repository: repository, RunID: 42, BatchID: "42-1",
				SourceSHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			})

			// Then: the plan fails closed before aggregation.
			require.ErrorIs(t, err, ErrInvalidPlan)
		})
	}
}

func writeFixture(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}
