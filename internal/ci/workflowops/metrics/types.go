package metrics

import (
	"errors"
	"fmt"
	"regexp"
)

var (
	ErrInvalidExpectedRun = errors.New("invalid expected metrics run")
	ErrInvalidMetrics     = errors.New("invalid metrics JSON")
)

var digestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

type ExpectedRun struct {
	id      int64
	attempt int64
}

func NewExpectedRun(id, attempt int64) (ExpectedRun, error) {
	if id < 1 || attempt < 1 {
		return ExpectedRun{}, fmt.Errorf("%w: id and attempt must be positive", ErrInvalidExpectedRun)
	}
	return ExpectedRun{id: id, attempt: attempt}, nil
}

func (run ExpectedRun) ID() int64 {
	return run.id
}

func (run ExpectedRun) Attempt() int64 {
	return run.attempt
}

type ValidationError struct {
	Path   string
	Reason string
}

func (err *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s: %v", err.Path, err.Reason, ErrInvalidMetrics)
}

func (err *ValidationError) Is(target error) bool {
	return target == ErrInvalidMetrics
}

type record struct {
	SchemaVersion string       `json:"schema_version"`
	Run           *runRecord   `json:"run"`
	Image         *imageRecord `json:"image"`
	Scan          *scanRecord  `json:"scan"`
	Platforms     *platforms   `json:"platforms"`
	SupplyChain   *supplyChain `json:"supply_chain"`
}

type runRecord struct {
	ID         int64  `json:"id"`
	Attempt    int64  `json:"attempt"`
	StartedAt  string `json:"started_at"`
	EndedAt    string `json:"ended_at"`
	Conclusion string `json:"conclusion"`
}

type imageRecord struct {
	Name           string  `json:"name"`
	SourceTag      string  `json:"source_tag"`
	TargetRef      *string `json:"target_ref"`
	ManifestDigest *string `json:"manifest_digest"`
}

type scanRecord struct {
	Before *scanSnapshot `json:"before"`
	After  *scanSnapshot `json:"after"`
}

type scanSnapshot struct {
	VulnerabilityCount int64          `json:"vuln_count"`
	BySeverity         *severityCount `json:"by_severity"`
}

type severityCount struct {
	Critical *int64 `json:"CRITICAL"`
	High     *int64 `json:"HIGH"`
	Medium   *int64 `json:"MEDIUM"`
	Low      *int64 `json:"LOW"`
	Unknown  *int64 `json:"UNKNOWN"`
}

type platforms struct {
	AMD64 *platform `json:"amd64"`
	ARM64 *platform `json:"arm64"`
}

type platform struct {
	Architecture        string  `json:"arch"`
	CopaDurationSeconds *int64  `json:"copa_duration_seconds"`
	CopaExitCode        *int64  `json:"copa_exit_code"`
	StagingDigest       *string `json:"staging_digest"`
}

type supplyChain struct {
	RekorURL              *string `json:"rekor_url"`
	AttestationID         *string `json:"attestation_id"`
	SBOMDigest            *string `json:"sbom_digest"`
	AttestationBundlePath *string `json:"attestation_bundle_path"`
}
