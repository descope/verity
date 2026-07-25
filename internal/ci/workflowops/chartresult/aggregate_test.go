package chartresult

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAggregate_accepts_exact_declared_results_and_identity(t *testing.T) {
	// Given an exact reusable-workflow identity and the complete declared result set.
	input := Input{
		Results: []string{"discover-charts=success", "chart-test=skipped"},
		Identity: IdentityInput{
			SourceSHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			RunID:     "42", RunAttempt: "3", PublicationID: "publication-42",
			BatchID: "batch-42", ArtifactName: "chart-publication-publication-42",
			ArtifactDigest: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		},
	}

	// When the chart result set is aggregated.
	result, err := Aggregate(&input)

	// Then only the seven exact small identity outputs are returned.
	require.NoError(t, err)
	assert.Equal(t, Result{
		SourceSHA: input.Identity.SourceSHA,
		RunID:     input.Identity.RunID, RunAttempt: input.Identity.RunAttempt,
		PublicationID: input.Identity.PublicationID, BatchID: input.Identity.BatchID,
		ArtifactName: input.Identity.ArtifactName, ArtifactDigest: input.Identity.ArtifactDigest,
	}, result)
}

func TestAggregate_accepts_exact_privileged_result_profile(t *testing.T) {
	// Given the privileged profile's single declared matrix result and exact identity.
	input := validInput([]string{"chart-test=success"})
	input.Profile = "privileged"

	// When the privileged chart result set is aggregated.
	result, err := Aggregate(&input)

	// Then the exact producer identity is emitted.
	require.NoError(t, err)
	assert.Equal(t, input.Identity.ArtifactDigest, result.ArtifactDigest)
}

func TestAggregate_rejects_failed_or_duplicate_privileged_results(t *testing.T) {
	tests := []struct {
		name    string
		results []string
	}{
		{name: "failed", results: []string{"chart-test=failure"}},
		{name: "duplicate", results: []string{"chart-test=success", "chart-test=success"}},
		{name: "unexpected normal shard", results: []string{"discover-charts=success", "chart-test=success"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given an inexact privileged result declaration.
			input := validInput(test.results)
			input.Profile = "privileged"

			// When aggregation validates the privileged profile.
			_, err := Aggregate(&input)

			// Then it fails closed.
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrInvalidResults)
		})
	}
}

func TestAggregate_rejects_malformed_missing_failed_or_duplicate_results(t *testing.T) {
	tests := []struct {
		name    string
		results []string
	}{
		{name: "malformed", results: []string{"discover-charts=success", "chart-test"}},
		{name: "missing", results: []string{"discover-charts=success"}},
		{name: "failed", results: []string{"discover-charts=success", "chart-test=failure"}},
		{name: "duplicate", results: []string{"discover-charts=success", "chart-test=success", "chart-test=skipped"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given an invalid declared chart result set.
			input := validInput(test.results)

			// When aggregation parses and validates the set.
			_, err := Aggregate(&input)

			// Then it fails closed without an identity result.
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrInvalidResults)
		})
	}
}

func TestAggregate_rejects_malformed_exact_identity(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*IdentityInput)
	}{
		{name: "wrong digest", mutate: func(input *IdentityInput) { input.ArtifactDigest = "sha256:wrong" }},
		{name: "oversized run id", mutate: func(input *IdentityInput) { input.RunID = "18446744073709551616" }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given successful declared results with a malformed exact identity.
			input := validInput([]string{"discover-charts=success", "chart-test=success"})
			test.mutate(&input.Identity)

			// When aggregation parses the identity boundary.
			_, err := Aggregate(&input)

			// Then malformed or oversized identity fails closed.
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrInvalidIdentity)
		})
	}
}

func validInput(results []string) Input {
	return Input{
		Results: results,
		Identity: IdentityInput{
			SourceSHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			RunID:     "42", RunAttempt: "3", PublicationID: "publication-42",
			BatchID: "batch-42", ArtifactName: "chart-publication-publication-42",
			ArtifactDigest: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		},
	}
}
