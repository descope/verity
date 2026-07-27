package patchimage

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestParseInputs_stripsDigestAndPreservesRegistryPort(t *testing.T) {
	// Given
	input := WorkflowInputs{
		SourceRef:      "localhost:5000/team/image:v1.2.3@sha256:aaaaaaaa",
		ImageName:      "team/image: release",
		TargetRegistry: "ghcr.io/verity-org",
	}

	// When
	result := ParseInputs(input)

	// Then
	assert.Equal(t, "v1.2.3", result.SourceTag)
	assert.Equal(t, "team-image--release", result.SafeName)
	assert.Equal(t, "ghcr.io/verity-org/cache", result.StagingRegistry)
}

func TestPlatformRequested_matchesLegacySubstringBehavior(t *testing.T) {
	// Given
	platforms := "linux/amd64,linux/arm64"

	// When / Then
	assert.True(t, PlatformRequested(platforms, "linux/amd64"))
	assert.False(t, PlatformRequested(platforms, "linux/s390x"))
}

func TestTrivyDateKey_usesUTCHour(t *testing.T) {
	// Given
	instant := time.Date(2026, time.July, 25, 23, 59, 0, 0, time.FixedZone("offset", 2*60*60))

	// When
	key := TrivyDateKey(instant)

	// Then
	assert.Equal(t, "2026-07-25-21", key)
}
