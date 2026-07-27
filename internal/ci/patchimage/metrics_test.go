package patchimage

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPlatformMetrics_mapsInvalidNumbersAndEmptyDigestToNull(t *testing.T) {
	// Given / When
	metrics := NewPlatformMetrics(PlatformMetricsInput{
		Arch: "amd64", Duration: "not-a-number", ExitCode: "7", Digest: "",
	})

	// Then
	assert.Equal(t, "amd64", metrics.Arch)
	assert.Nil(t, metrics.DurationSeconds)
	require.NotNil(t, metrics.ExitCode)
	assert.Equal(t, int64(7), *metrics.ExitCode)
	assert.Nil(t, metrics.StagingDigest)
}

func TestBuildSuccessMetrics_preservesSchemaAndDerivesFailureFromLateOutcome(t *testing.T) {
	// Given
	before, err := ParseTrivyReport([]byte(trivyReportFixture))
	require.NoError(t, err)
	after, err := ParseTrivyReport([]byte(`{"Results":[]}`))
	require.NoError(t, err)
	endedAt := time.Date(2026, time.July, 25, 12, 0, 0, 0, time.UTC)
	input := SuccessMetricsInput{
		Run:    RunMetrics{ID: 42, Attempt: 3, StartedAt: "2026-07-25T11:00:00Z", EndedAt: endedAt},
		Image:  ImageMetrics{Name: "nginx", SourceTag: "1.29.3", TargetRef: "ghcr.io/verity/nginx:1.29.3", ManifestDigest: "sha256:abc"},
		Before: before, After: after,
		Outcomes:    []string{"success", "skipped", "failure"},
		Platforms:   PlatformSet{AMD64: json.RawMessage(`{"arch":"amd64"}`), ARM64: json.RawMessage(`null`)},
		SupplyChain: SupplyChainInput{SBOMDigest: "abc123"},
	}

	// When
	document := BuildSuccessMetrics(&input)

	// Then
	assert.Equal(t, "v1", document.SchemaVersion)
	assert.Equal(t, "failure", document.Run.Conclusion)
	assert.Equal(t, 3, document.Scan.Before.VulnerabilityCount)
	assert.Equal(t, 0, document.Scan.After.VulnerabilityCount)
	require.NotNil(t, document.SupplyChain.SBOMDigest)
	assert.Equal(t, "sha256:abc123", *document.SupplyChain.SBOMDigest)
	encoded, err := json.Marshal(document)
	require.NoError(t, err)
	assert.JSONEq(t, `{
	  "schema_version":"v1",
	  "run":{"id":42,"attempt":3,"started_at":"2026-07-25T11:00:00Z","ended_at":"2026-07-25T12:00:00Z","conclusion":"failure"},
	  "image":{"name":"nginx","source_tag":"1.29.3","target_ref":"ghcr.io/verity/nginx:1.29.3","manifest_digest":"sha256:abc"},
	  "scan":{
	    "before":{"vuln_count":3,"by_severity":{"CRITICAL":1,"HIGH":2,"MEDIUM":0,"LOW":0,"UNKNOWN":0}},
	    "after":{"vuln_count":0,"by_severity":{"CRITICAL":0,"HIGH":0,"MEDIUM":0,"LOW":0,"UNKNOWN":0}}
	  },
	  "platforms":{"amd64":{"arch":"amd64"},"arm64":null},
	  "supply_chain":{"rekor_url":null,"attestation_id":null,"sbom_digest":"sha256:abc123","attestation_bundle_path":null}
	}`, string(encoded))
}

func TestBuildFailureMetrics_keepsScanNullAndPlatformEvidence(t *testing.T) {
	// Given
	input := FailureMetricsInput{
		Run:       RunMetrics{ID: 9, Attempt: 2, StartedAt: "2026-07-25T10:00:00Z", EndedAt: time.Date(2026, time.July, 25, 10, 5, 0, 0, time.UTC)},
		ImageName: "nginx", SourceTag: "1.29.3",
		Platforms: PlatformSet{AMD64: json.RawMessage(`{"arch":"amd64","copa_exit_code":1}`), ARM64: json.RawMessage(`null`)},
	}

	// When
	document := BuildFailureMetrics(&input)

	// Then
	assert.Equal(t, "failure", document.Run.Conclusion)
	assert.Nil(t, document.Scan.Before)
	assert.Nil(t, document.Scan.After)
	assert.JSONEq(t, `{"arch":"amd64","copa_exit_code":1}`, string(document.Platforms.AMD64))
	encoded, err := json.Marshal(document)
	require.NoError(t, err)
	assert.JSONEq(t, `{
	  "schema_version":"v1",
	  "run":{"id":9,"attempt":2,"started_at":"2026-07-25T10:00:00Z","ended_at":"2026-07-25T10:05:00Z","conclusion":"failure"},
	  "image":{"name":"nginx","source_tag":"1.29.3","target_ref":null,"manifest_digest":null},
	  "scan":{"before":null,"after":null},
	  "platforms":{"amd64":{"arch":"amd64","copa_exit_code":1},"arm64":null},
	  "supply_chain":{"rekor_url":null,"attestation_id":null,"sbom_digest":null,"attestation_bundle_path":null}
	}`, string(encoded))
}
