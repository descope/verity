package cmd

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPRTrivyCacheValues_parse_version_and_use_UTC_hour(t *testing.T) {
	// Given: Trivy's version output and a time with a non-UTC offset.
	now := time.Date(2026, time.July, 25, 14, 42, 0, 0, time.FixedZone("offset", 2*60*60))

	// When: the typed cache metadata is derived.
	values, err := prTrivyCacheValues("Version: 0.65.0\nVulnerability DB:\n", now)

	// Then: the exact version and UTC hour are emitted without shell parsing.
	require.NoError(t, err)
	require.Equal(t, [][2]string{{"date", "2026-07-25-12"}, {"version", "0.65.0"}}, values)
}

func TestPRTrivyCacheValues_reject_malformed_version_output(t *testing.T) {
	// Given: output that does not contain Trivy's version header.

	// When: cache metadata is parsed.
	_, err := prTrivyCacheValues("Trivy development build\n", time.Now())

	// Then: the cache key fails closed instead of sharing an unversioned database.
	require.ErrorIs(t, err, errPRCommandFailed)
}
