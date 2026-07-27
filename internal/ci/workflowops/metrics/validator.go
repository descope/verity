package metrics

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/verity-org/verity/internal/ci/workflowops/strictjson"
)

type ValidationResult struct {
	Count     int
	Artifacts []ValidatedArtifact
}

type ValidatedArtifact struct {
	Path   string
	Digest [sha256.Size]byte
}

func ValidateDirectory(ctx context.Context, expected ExpectedRun, dir string) (ValidationResult, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return ValidationResult{}, fmt.Errorf("stat metrics directory %q: %w", dir, err)
	}
	if !info.IsDir() {
		return ValidationResult{}, fmt.Errorf("metrics path %q is not a directory: %w", dir, ErrInvalidMetrics)
	}

	files, err := filepath.Glob(filepath.Join(dir, "metrics-*.json"))
	if err != nil {
		return ValidationResult{}, fmt.Errorf("glob metrics directory %q: %w", dir, err)
	}
	if len(files) == 0 {
		return ValidationResult{}, fmt.Errorf("no metrics artifacts in %q: %w", dir, ErrInvalidMetrics)
	}
	sort.Strings(files)

	artifacts := make([]ValidatedArtifact, 0, len(files))
	for _, path := range files {
		if err := ctx.Err(); err != nil {
			return ValidationResult{}, err
		}
		artifact, err := validateFile(path, expected)
		if err != nil {
			return ValidationResult{}, err
		}
		artifacts = append(artifacts, artifact)
	}
	return ValidationResult{Count: len(artifacts), Artifacts: artifacts}, nil
}

func validateFile(path string, expected ExpectedRun) (ValidatedArtifact, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return ValidatedArtifact{}, fmt.Errorf("read metrics file %q: %w", path, err)
	}
	var value record
	if err := strictjson.Decode(content, &value); err != nil {
		return ValidatedArtifact{}, &ValidationError{Path: path, Reason: "decode JSON: " + err.Error()}
	}
	if reason := validateRecord(value, expected); reason != "" {
		return ValidatedArtifact{}, &ValidationError{Path: path, Reason: reason}
	}
	return ValidatedArtifact{Path: path, Digest: sha256.Sum256(content)}, nil
}

func validateRecord(value record, expected ExpectedRun) string {
	if value.SchemaVersion != "v1" {
		return "schema_version must be v1"
	}
	if value.Run == nil || value.Image == nil || value.Scan == nil || value.Platforms == nil || value.SupplyChain == nil {
		return "required object is missing"
	}
	if value.Run.ID != expected.id || value.Run.Attempt != expected.attempt {
		return "run identity does not match"
	}
	if reason := validateIdentity(value); reason != "" {
		return reason
	}
	return validateRecordData(value)
}

func validateIdentity(value record) string {
	if value.Run.StartedAt == "" || value.Run.EndedAt == "" || !validConclusion(value.Run.Conclusion) {
		return "run metadata is invalid"
	}
	if value.Image.Name == "" || value.Image.SourceTag == "" {
		return "image identity is invalid"
	}
	return ""
}

func validateRecordData(value record) string {
	if reason := validateConclusionData(value); reason != "" {
		return reason
	}
	if reason := validatePlatform(value.Platforms.AMD64, "amd64"); reason != "" {
		return reason
	}
	if reason := validatePlatform(value.Platforms.ARM64, "arm64"); reason != "" {
		return reason
	}
	return validateSupplyChain(value.SupplyChain)
}

func validConclusion(value string) bool {
	switch value {
	case "success", "failure", "cancelled", "skipped":
		return true
	default:
		return false
	}
}

func validateConclusionData(value record) string {
	if value.Run.Conclusion == "success" {
		if !validString(value.Image.TargetRef) || !validDigest(value.Image.ManifestDigest) {
			return "successful image target is invalid"
		}
		if !validScan(value.Scan.Before) || !validScan(value.Scan.After) {
			return "successful scan is invalid"
		}
		return ""
	}
	if !validOptionalString(value.Image.TargetRef) || !validOptionalDigest(value.Image.ManifestDigest) {
		return "failed image target is invalid"
	}
	if value.Scan.Before != nil && !validScan(value.Scan.Before) {
		return "before scan is invalid"
	}
	if value.Scan.After != nil && !validScan(value.Scan.After) {
		return "after scan is invalid"
	}
	return ""
}

func validScan(value *scanSnapshot) bool {
	if value == nil || value.VulnerabilityCount < 0 || value.BySeverity == nil {
		return false
	}
	counts := []*int64{value.BySeverity.Critical, value.BySeverity.High, value.BySeverity.Medium, value.BySeverity.Low, value.BySeverity.Unknown}
	var total int64
	for _, count := range counts {
		if count == nil || *count < 0 {
			return false
		}
		total += *count
	}
	return total == value.VulnerabilityCount
}

func validatePlatform(value *platform, architecture string) string {
	if value == nil {
		return ""
	}
	if value.Architecture != architecture {
		return architecture + " platform architecture is invalid"
	}
	if value.CopaDurationSeconds != nil && *value.CopaDurationSeconds < 0 {
		return architecture + " platform duration is invalid"
	}
	if !validOptionalDigest(value.StagingDigest) {
		return architecture + " staging digest is invalid"
	}
	return ""
}

func validateSupplyChain(value *supplyChain) string {
	strings := []*string{value.RekorURL, value.AttestationID, value.AttestationBundlePath}
	for _, item := range strings {
		if !validOptionalString(item) {
			return "supply-chain string is invalid"
		}
	}
	if !validOptionalDigest(value.SBOMDigest) {
		return "supply-chain digest is invalid"
	}
	return ""
}

func validString(value *string) bool {
	return value != nil && *value != ""
}

func validOptionalString(value *string) bool {
	return value == nil || *value != ""
}

func validDigest(value *string) bool {
	return value != nil && digestPattern.MatchString(*value)
}

func validOptionalDigest(value *string) bool {
	return value == nil || digestPattern.MatchString(*value)
}
