package main

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseStrictBoolean_accepts_only_lowercase_literals(t *testing.T) {
	for value, expected := range map[string]bool{"false": false, "true": true} {
		t.Run(value, func(t *testing.T) {
			// Given an exact lowercase boolean literal.

			// When the action boundary parses it.
			actual, err := parseStrictBoolean(value)

			// Then the typed value is accepted.
			require.NoError(t, err)
			assert.Equal(t, expected, actual)
		})
	}
}

func TestRunVerifyRemote_rejects_noncanonical_boolean_before_API(t *testing.T) {
	// Given otherwise valid current-run identity and a Go boolean alias.
	name := "verity-linux-amd64-" + testActionBuildKey + "-42-2"
	digest := "sha256:" + strings.Repeat("b", 64)
	calls := 0
	server := fakeArtifactServer(t, name, digest, testActionSourceSHA, &calls)
	t.Setenv("GITHUB_API_URL", server.URL)
	t.Setenv("GH_TOKEN", "token")

	// When the complete verify-remote command boundary receives protected-attestation=1.
	err := runVerifyRemote(context.Background(), []string{
		"--artifact-name", name,
		"--artifact-digest", digest,
		"--source-sha", testActionSourceSHA,
		"--build-key", testActionBuildKey,
		"--repository", "verity-org/verity",
		"--run-id", "42",
		"--run-attempt", "2",
		"--protected-attestation", "1",
	})

	// Then the alias fails before helper protected behavior or any API request.
	require.Error(t, err)
	assert.ErrorIs(t, err, errUntrustedArtifact)
	assert.Zero(t, calls)
}

func TestRunVerifyRemote_accepts_protected_producer_before_attestation_exists(t *testing.T) {
	// Given the protected producer emitted an exact artifact that has not yet been attested.
	name := "verity-linux-amd64-" + testActionBuildKey + "-42-2"
	digest := "sha256:" + strings.Repeat("b", 64)
	server := fakeArtifactServer(t, name, digest, testActionSourceSHA, nil)
	t.Setenv("GITHUB_API_URL", server.URL)
	t.Setenv("GH_TOKEN", "token")

	// When bootstrap recovery verifies protected producer identity without requiring its future attestation.
	err := runVerifyRemote(context.Background(), []string{
		"--artifact-name", name,
		"--artifact-digest", digest,
		"--source-sha", testActionSourceSHA,
		"--build-key", testActionBuildKey,
		"--repository", "verity-org/verity",
		"--run-id", "42",
		"--run-attempt", "2",
		"--protected-producer", "true",
		"--protected-attestation", "false",
	})

	// Then the circular attestation dependency is avoided without accepting the unprotected producer.
	require.NoError(t, err)
}

func TestParseStrictBoolean_rejects_aliases_case_and_whitespace_variants(t *testing.T) {
	for _, value := range []string{
		"", "0", "1", "FALSE", "False", "TRUE", "True", "no", "yes",
		" false", "false ", "\ttrue", "true\n",
	} {
		t.Run(value, func(t *testing.T) {
			// Given a noncanonical boolean spelling.

			// When the action boundary parses it.
			_, err := parseStrictBoolean(value)

			// Then protected behavior cannot be enabled or disabled ambiguously.
			require.Error(t, err)
			assert.ErrorIs(t, err, errUntrustedArtifact)
		})
	}
}
