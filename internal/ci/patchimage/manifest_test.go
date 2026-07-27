package patchimage

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildManifestPlan_derivesEveryPlatformTag(t *testing.T) {
	// Given
	input := ManifestPlanInput{
		ImageName: "library/nginx", SourceTag: "1.29.3",
		StagingRegistry: "ghcr.io/verity/cache", Platforms: "linux/amd64,linux/arm64",
	}

	// When
	plan, err := BuildManifestPlan(input)

	// Then
	require.NoError(t, err)
	assert.Equal(t, "ghcr.io/verity/cache:library-nginx-1.29.3", plan.ManifestTag)
	assert.Equal(t, []string{
		"ghcr.io/verity/cache:library-nginx-1.29.3-amd64",
		"ghcr.io/verity/cache:library-nginx-1.29.3-arm64",
	}, plan.SourceTags)
}

func TestShouldPublish_requiresPreviousReportSameSetAndExistingTargetForNoOp(t *testing.T) {
	// Given
	current, err := ParseTrivyReport([]byte(`{"Results":[{"Vulnerabilities":[{"VulnerabilityID":"CVE-1"}]}]}`))
	require.NoError(t, err)
	previous, err := ParseTrivyReport([]byte(`{"Results":[{"Vulnerabilities":[{"VulnerabilityID":"CVE-1"}]}]}`))
	require.NoError(t, err)

	// When / Then
	assert.False(t, ShouldPublish(&CompareInput{PreviousExisted: true, TargetExists: true, Current: current, Previous: previous}))
	assert.True(t, ShouldPublish(&CompareInput{PreviousExisted: false, TargetExists: true, Current: current, Previous: previous}))
	assert.True(t, ShouldPublish(&CompareInput{PreviousExisted: true, TargetExists: false, Current: current, Previous: previous}))
}

func TestExtractRekorURL_prefersBundleLogIndexThenFallsBackToOutput(t *testing.T) {
	// Given / When
	fromBundle := ExtractRekorURL([]byte(`{"verificationMaterial":{"tlogEntries":[{"logIndex":42}]}}`), nil)
	fromOutput := ExtractRekorURL(nil, []byte("entry https://rekor.sigstore.dev/api/v1/log/entries/abc\n"))

	// Then
	assert.Equal(t, "https://rekor.sigstore.dev/api/v1/log/entries?logIndex=42", fromBundle)
	assert.Equal(t, "https://rekor.sigstore.dev/api/v1/log/entries/abc", fromOutput)
}
