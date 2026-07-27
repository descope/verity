package patchimage

import (
	"encoding/json"
	"maps"
	"strconv"
	"time"
)

type PlatformMetricsInput struct {
	Arch     string
	Duration string
	ExitCode string
	Digest   string
}

type PlatformMetrics struct {
	Arch            string  `json:"arch"`
	DurationSeconds *int64  `json:"copa_duration_seconds"`
	ExitCode        *int64  `json:"copa_exit_code"`
	StagingDigest   *string `json:"staging_digest"`
}

func NewPlatformMetrics(input PlatformMetricsInput) PlatformMetrics {
	return PlatformMetrics{
		Arch:            input.Arch,
		DurationSeconds: optionalInt64(input.Duration),
		ExitCode:        optionalInt64(input.ExitCode),
		StagingDigest:   optionalString(input.Digest),
	}
}

type RunMetrics struct {
	ID        int64
	Attempt   int64
	StartedAt string
	EndedAt   time.Time
}

type ImageMetrics struct {
	Name           string
	SourceTag      string
	TargetRef      string
	ManifestDigest string
}

type PlatformSet struct {
	AMD64 json.RawMessage `json:"amd64"`
	ARM64 json.RawMessage `json:"arm64"`
}

type SupplyChainInput struct {
	RekorURL              string
	AttestationID         string
	SBOMDigest            string
	AttestationBundlePath string
}

type SuccessMetricsInput struct {
	Run         RunMetrics
	Image       ImageMetrics
	Before      TrivySummary
	After       TrivySummary
	Outcomes    []string
	Platforms   PlatformSet
	SupplyChain SupplyChainInput
}

type FailureMetricsInput struct {
	Run       RunMetrics
	ImageName string
	SourceTag string
	Platforms PlatformSet
}

type MetricsDocument struct {
	SchemaVersion string             `json:"schema_version"`
	Run           metricsRun         `json:"run"`
	Image         metricsImage       `json:"image"`
	Scan          metricsScan        `json:"scan"`
	Platforms     PlatformSet        `json:"platforms"`
	SupplyChain   metricsSupplyChain `json:"supply_chain"`
}

type metricsRun struct {
	ID         int64  `json:"id"`
	Attempt    int64  `json:"attempt"`
	StartedAt  string `json:"started_at"`
	EndedAt    string `json:"ended_at"`
	Conclusion string `json:"conclusion"`
}

type metricsImage struct {
	Name           string  `json:"name"`
	SourceTag      string  `json:"source_tag"`
	TargetRef      *string `json:"target_ref"`
	ManifestDigest *string `json:"manifest_digest"`
}

type metricsScan struct {
	Before *scanSnapshot `json:"before"`
	After  *scanSnapshot `json:"after"`
}

type scanSnapshot struct {
	VulnerabilityCount int            `json:"vuln_count"`
	BySeverity         map[string]int `json:"by_severity"`
}

type metricsSupplyChain struct {
	RekorURL              *string `json:"rekor_url"`
	AttestationID         *string `json:"attestation_id"`
	SBOMDigest            *string `json:"sbom_digest"`
	AttestationBundlePath *string `json:"attestation_bundle_path"`
}

func BuildSuccessMetrics(input *SuccessMetricsInput) MetricsDocument {
	conclusion := "success"
	for _, outcome := range input.Outcomes {
		if outcome == "failure" {
			conclusion = "failure"
		}
	}
	if input.Image.TargetRef == "" || input.Image.ManifestDigest == "" {
		conclusion = "failure"
	}
	sbomDigest := input.SupplyChain.SBOMDigest
	if sbomDigest != "" {
		sbomDigest = "sha256:" + sbomDigest
	}
	return MetricsDocument{
		SchemaVersion: "v1",
		Run:           buildRun(input.Run, conclusion),
		Image: metricsImage{
			Name: input.Image.Name, SourceTag: input.Image.SourceTag,
			TargetRef: optionalString(input.Image.TargetRef), ManifestDigest: optionalString(input.Image.ManifestDigest),
		},
		Scan: metricsScan{
			Before: &scanSnapshot{VulnerabilityCount: input.Before.Count, BySeverity: cloneCounts(input.Before.BySeverity)},
			After:  &scanSnapshot{VulnerabilityCount: input.After.Count, BySeverity: cloneCounts(input.After.BySeverity)},
		},
		Platforms: input.Platforms,
		SupplyChain: metricsSupplyChain{
			RekorURL: optionalString(input.SupplyChain.RekorURL), AttestationID: optionalString(input.SupplyChain.AttestationID),
			SBOMDigest: optionalString(sbomDigest), AttestationBundlePath: optionalString(input.SupplyChain.AttestationBundlePath),
		},
	}
}

func BuildFailureMetrics(input *FailureMetricsInput) MetricsDocument {
	return MetricsDocument{
		SchemaVersion: "v1",
		Run:           buildRun(input.Run, "failure"),
		Image:         metricsImage{Name: input.ImageName, SourceTag: input.SourceTag},
		Scan:          metricsScan{},
		Platforms:     input.Platforms,
		SupplyChain:   metricsSupplyChain{},
	}
}

func buildRun(input RunMetrics, conclusion string) metricsRun {
	return metricsRun{
		ID: input.ID, Attempt: input.Attempt, StartedAt: input.StartedAt,
		EndedAt: input.EndedAt.UTC().Format(time.RFC3339), Conclusion: conclusion,
	}
}

func optionalInt64(value string) *int64 {
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return nil
	}
	return &parsed
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func cloneCounts(input map[string]int) map[string]int {
	result := make(map[string]int, len(input))
	maps.Copy(result, input)
	return result
}
