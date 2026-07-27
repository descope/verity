package repositoryops

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
)

const maxTrivyReportBytes = 64 << 20

var (
	ErrMalformedTrivyReport        = errors.New("malformed Trivy report")
	ErrFixableNonGoVulnerabilities = errors.New("fixable non-Go vulnerabilities remain")
	ErrInvalidVulnerabilityCounts  = errors.New("invalid vulnerability counts")
	ErrInvalidImageLabel           = errors.New("invalid image label")
	ErrInvalidFilePath             = errors.New("invalid file path")
)

type VulnerabilityCounts struct {
	Total int
	Go    int
	NonGo int
}

func (c VulnerabilityCounts) validate() error {
	if c.Total < 0 || c.Go < 0 || c.NonGo < 0 || c.Total != c.Go+c.NonGo {
		return fmt.Errorf("%w: total=%d go=%d non-go=%d", ErrInvalidVulnerabilityCounts, c.Total, c.Go, c.NonGo)
	}
	return nil
}

type ScanBeforeInput struct {
	Source     string
	ReportPath string
}

type ScanBeforeRequest struct {
	source     string
	reportPath string
}

func NewScanBeforeRequest(input ScanBeforeInput) (ScanBeforeRequest, error) {
	source, err := validatedImageReference(input.Source)
	if err != nil {
		return ScanBeforeRequest{}, err
	}
	reportPath, err := validatedPath("Trivy report", input.ReportPath)
	if err != nil {
		return ScanBeforeRequest{}, err
	}
	return ScanBeforeRequest{source: source, reportPath: reportPath}, nil
}

type VerifyPatchedInput struct {
	Image      string
	ImageLabel string
	ReportPath string
	Before     VulnerabilityCounts
}

type VerifyPatchedRequest struct {
	image      string
	imageLabel string
	reportPath string
	before     VulnerabilityCounts
}

func NewVerifyPatchedRequest(input VerifyPatchedInput) (VerifyPatchedRequest, error) {
	image, err := validatedImageReference(input.Image)
	if err != nil {
		return VerifyPatchedRequest{}, err
	}
	label := strings.TrimSpace(input.ImageLabel)
	if label == "" || containsControl(label) {
		return VerifyPatchedRequest{}, ErrInvalidImageLabel
	}
	reportPath, err := validatedPath("Trivy report", input.ReportPath)
	if err != nil {
		return VerifyPatchedRequest{}, err
	}
	if err := input.Before.validate(); err != nil {
		return VerifyPatchedRequest{}, err
	}
	return VerifyPatchedRequest{image: image, imageLabel: label, reportPath: reportPath, before: input.Before}, nil
}

type ScanService struct {
	Commands CommandRunner
}

type ScanBeforeResult struct {
	Counts    VulnerabilityCounts
	SkipPatch bool
}

func (s ScanService) Before(ctx context.Context, request ScanBeforeRequest) (ScanBeforeResult, error) {
	commands := s.Commands
	if commands == nil {
		commands = ExecCommandRunner{}
	}
	if err := prepareTrivyReport(request.reportPath); err != nil {
		return ScanBeforeResult{}, err
	}
	if _, err := runRequiredCommand(ctx, commands, &Command{
		Name: "trivy",
		Args: []string{
			"image", "--severity", "CRITICAL,HIGH,MEDIUM,LOW", "--scanners", "vuln",
			"--format", "json", "--output", request.reportPath, request.source,
		},
	}); err != nil {
		return ScanBeforeResult{}, fmt.Errorf("scan source image: %w", err)
	}
	counts, err := readTrivyCounts(request.reportPath)
	if err != nil {
		return ScanBeforeResult{}, err
	}
	return ScanBeforeResult{Counts: counts, SkipPatch: counts.Total == 0}, nil
}

type VerifyPatchedResult struct {
	Before VulnerabilityCounts
	After  VulnerabilityCounts
}

func (s ScanService) Verify(ctx context.Context, request VerifyPatchedRequest) (VerifyPatchedResult, error) {
	commands := s.Commands
	if commands == nil {
		commands = ExecCommandRunner{}
	}
	if err := prepareTrivyReport(request.reportPath); err != nil {
		return VerifyPatchedResult{}, err
	}
	if _, err := runRequiredCommand(ctx, commands, &Command{
		Name: "trivy",
		Args: []string{
			"image", "--severity", "CRITICAL,HIGH,MEDIUM,LOW", "--ignore-unfixed", "--scanners", "vuln",
			"--format", "json", "--output", request.reportPath, request.image,
		},
	}); err != nil {
		return VerifyPatchedResult{}, fmt.Errorf("scan patched image: %w", err)
	}
	after, err := readTrivyCounts(request.reportPath)
	if err != nil {
		return VerifyPatchedResult{}, err
	}
	result := VerifyPatchedResult{Before: request.before, After: after}
	if after.NonGo != 0 {
		return result, fmt.Errorf("%w: %s has %d non-Go and %d Go vulnerabilities", ErrFixableNonGoVulnerabilities, request.imageLabel, after.NonGo, after.Go)
	}
	return result, nil
}

func readTrivyCounts(path string) (VulnerabilityCounts, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return VulnerabilityCounts{}, fmt.Errorf("%w: inspect report %q: %w", ErrMalformedTrivyReport, path, err)
	}
	if !info.Mode().IsRegular() {
		return VulnerabilityCounts{}, fmt.Errorf("%w: report %q is not a regular file", ErrMalformedTrivyReport, path)
	}
	if info.Size() > maxTrivyReportBytes {
		return VulnerabilityCounts{}, fmt.Errorf("%w: report %q exceeds %d bytes", ErrMalformedTrivyReport, path, maxTrivyReportBytes)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return VulnerabilityCounts{}, fmt.Errorf("read Trivy report %q: %w", path, err)
	}
	results, err := parseTrivyResults(data)
	if err != nil {
		return VulnerabilityCounts{}, err
	}
	counts := VulnerabilityCounts{}
	for _, result := range results {
		counts.Total += result.vulnerabilityCount
		if result.Type == "gobinary" {
			counts.Go += result.vulnerabilityCount
		} else {
			counts.NonGo += result.vulnerabilityCount
		}
	}
	return counts, nil
}

func prepareTrivyReport(path string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect existing Trivy report %q: %w", path, err)
	}
	if info.IsDir() {
		return fmt.Errorf("%w: report %q is a directory", ErrInvalidFilePath, path)
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("remove stale Trivy report %q: %w", path, err)
	}
	return nil
}

func validatedImageReference(value string) (string, error) {
	value = strings.TrimSpace(value)
	if _, err := name.ParseReference(value, name.StrictValidation); err != nil {
		return "", fmt.Errorf("invalid image reference %q: %w", value, err)
	}
	return value, nil
}

func validatedPath(label, value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || containsControl(value) {
		return "", fmt.Errorf("%w: %s path is empty or contains control characters", ErrInvalidFilePath, label)
	}
	return value, nil
}
