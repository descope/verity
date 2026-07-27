package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEvaluatePRAggregate_accepts_skipped_empty_smoke_matrix(t *testing.T) {
	// Given: one strict build, no smoke-only entries, and both native build markers.
	dir := t.TempDir()
	for _, arch := range []string{"amd64", "arm64"} {
		path := filepath.Join(dir, "build-demo-1-default-"+arch+".passed")
		require.NoError(t, os.WriteFile(path, nil, 0o600))
	}
	input := successfulPRAggregateInput(dir)
	input.IntegerSmokeResult = "skipped"
	input.ExpectedIntegerSmokeMatrix = `{"include":[]}`

	// When: final aggregation evaluates the exact expected matrices.
	err := evaluatePRAggregate(&input)

	// Then: empty smoke work may skip while strict dual-architecture evidence remains mandatory.
	require.NoError(t, err)
}

func TestEvaluatePRAggregate_rejects_missing_arm64_marker(t *testing.T) {
	// Given: only the amd64 strict-build marker exists.
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "build-demo-1-default-amd64.passed"), nil, 0o600))
	input := successfulPRAggregateInput(dir)
	input.IntegerSmokeResult = "skipped"
	input.ExpectedIntegerSmokeMatrix = `{"include":[]}`

	// When: final aggregation checks native coverage.
	err := evaluatePRAggregate(&input)

	// Then: the absent architecture fails the required PR result.
	require.ErrorContains(t, err, "missing successful Integer build security leg: demo:1-default (arm64)")
}

func TestEvaluatePRAggregate_rejects_non_successful_required_build(t *testing.T) {
	// Given: complete markers but a skipped required build matrix job.
	dir := t.TempDir()
	for _, kind := range []string{"smoke", "build"} {
		for _, arch := range []string{"amd64", "arm64"} {
			path := filepath.Join(dir, kind+"-demo-1-default-"+arch+".passed")
			require.NoError(t, os.WriteFile(path, nil, 0o600))
		}
	}
	input := successfulPRAggregateInput(dir)
	input.IntegerBuildResult = "skipped"

	// When: final aggregation evaluates required job outcomes.
	err := evaluatePRAggregate(&input)

	// Then: marker files cannot override an unsuccessful job result.
	require.ErrorContains(t, err, "integer-build-changed did not succeed: skipped")
}

func TestEvaluatePRAggregate_rejects_malformed_expected_matrix(t *testing.T) {
	// Given: an invalid expected build matrix from a required planner output.
	input := successfulPRAggregateInput(t.TempDir())
	input.ExpectedIntegerMatrix = `{`

	// When: final aggregation parses the typed matrix boundary.
	err := evaluatePRAggregate(&input)

	// Then: malformed evidence fails closed.
	require.ErrorContains(t, err, "invalid expected Integer build matrix")
}

func successfulPRAggregateInput(dir string) prAggregateInput {
	return prAggregateInput{
		ChangesResult: "success", Integer: true, Copa: false,
		DiscoverResult: "success", ValidateResult: "success", DetectIntegerResult: "success",
		IntegerHasChanges: true, IntegerSmokeResult: "success", IntegerBuildResult: "success",
		ExpectedIntegerMatrix:      `{"include":[{"image":"demo","version":"1","type":"default"}]}`,
		ExpectedIntegerSmokeMatrix: `{"include":[{"image":"demo","version":"1","type":"default"}]}`,
		DetectCopaResult:           "success", CopaChangedResult: "success", CopaRegressionResult: "success",
		SecurityDir: dir,
	}
}
