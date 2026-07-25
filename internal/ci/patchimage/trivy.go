package patchimage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/verity-org/verity/internal/ci/workflowops/retry"
)

var ErrMalformedTrivyReport = errors.New("malformed Trivy report")

type trivyReport struct {
	Results []trivyResult `json:"Results"`
}

type trivyResult struct {
	Vulnerabilities []trivyVulnerability `json:"Vulnerabilities"`
}

type trivyVulnerability struct {
	ID          string `json:"VulnerabilityID"`
	Severity    string `json:"Severity"`
	PackageName string `json:"PkgName"`
}

type TrivySummary struct {
	Count       int
	BySeverity  map[string]int
	identifiers []string
	packages    []string
}

func ParseTrivyReport(content []byte) (TrivySummary, error) {
	decoder := json.NewDecoder(bytes.NewReader(content))
	var report trivyReport
	if err := decoder.Decode(&report); err != nil {
		return TrivySummary{}, fmt.Errorf("%w: %w", ErrMalformedTrivyReport, err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return TrivySummary{}, err
	}
	summary := TrivySummary{BySeverity: baseSeverityCounts()}
	for _, result := range report.Results {
		for _, vulnerability := range result.Vulnerabilities {
			summary.Count++
			severity := vulnerability.Severity
			if severity == "" {
				severity = "UNKNOWN"
			}
			summary.BySeverity[severity]++
			summary.identifiers = append(summary.identifiers, vulnerability.ID)
			if vulnerability.PackageName != "" {
				summary.packages = append(summary.packages, vulnerability.PackageName)
			}
		}
	}
	return summary, nil
}

func ReadTrivyReport(path string) (TrivySummary, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return TrivySummary{}, fmt.Errorf("read Trivy report %q: %w", path, err)
	}
	return ParseTrivyReport(content)
}

func (summary TrivySummary) VulnerabilityFingerprint() string {
	values := append([]string(nil), summary.identifiers...)
	sort.Strings(values)
	var content strings.Builder
	for _, value := range values {
		if value == "" {
			value = "null"
		}
		content.WriteString(value)
		content.WriteByte('\n')
	}
	return sha256Hex([]byte(content.String()))
}

func (summary TrivySummary) PackageFingerprint() string {
	values := append([]string(nil), summary.packages...)
	sort.Strings(values)
	values = compactStrings(values)
	content, err := json.Marshal(values)
	if err != nil {
		return "unknown"
	}
	content = append(content, '\n')
	return sha256Hex(content)
}

type ScanRequest struct {
	Image      string
	ReportPath string
}

type ExistingImageRequest struct {
	Image      string
	ReportPath string
}

type ExistingImageResult struct {
	NeedsPatch bool
	Count      int
}

type ScanService struct {
	Runner retry.Runner
}

func (service ScanService) Scan(ctx context.Context, request ScanRequest) (TrivySummary, error) {
	if err := prepareReportPath(request.ReportPath); err != nil {
		return TrivySummary{}, err
	}
	if _, err := service.runner().Run(ctx, trivyCommand(request.Image, request.ReportPath)); err != nil {
		return TrivySummary{}, fmt.Errorf("scan image %q: %w", request.Image, err)
	}
	return ReadTrivyReport(request.ReportPath)
}

func (service ScanService) CheckExisting(ctx context.Context, request ExistingImageRequest) (ExistingImageResult, error) {
	if !commandSucceeded(ctx, service.runner(), &retry.Command{Name: "crane", Args: []string{"digest", request.Image}}) {
		return ExistingImageResult{NeedsPatch: true}, nil
	}
	if err := prepareReportPath(request.ReportPath); err != nil {
		return ExistingImageResult{}, err
	}
	if !commandSucceeded(ctx, service.runner(), trivyCommand(request.Image, request.ReportPath)) {
		return ExistingImageResult{NeedsPatch: true}, nil
	}
	summary, valid := readValidTrivyReport(request.ReportPath)
	if !valid {
		return ExistingImageResult{NeedsPatch: true}, nil
	}
	return ExistingImageResult{NeedsPatch: summary.Count > 0, Count: summary.Count}, nil
}

func commandSucceeded(ctx context.Context, runner retry.Runner, command *retry.Command) bool {
	_, err := runner.Run(ctx, command)
	return err == nil
}

func readValidTrivyReport(path string) (TrivySummary, bool) {
	summary, err := ReadTrivyReport(path)
	return summary, err == nil
}

func (service ScanService) runner() retry.Runner {
	if service.Runner != nil {
		return service.Runner
	}
	return retry.ExecRunner{}
}

func trivyCommand(image, reportPath string) *retry.Command {
	return &retry.Command{Name: "trivy", Args: []string{
		"image", "--vuln-type", "os,library", "--format", "json", "--quiet", "--output", reportPath, image,
	}}
}

func prepareReportPath(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove stale Trivy report %q: %w", path, err)
	}
	return nil
}

func baseSeverityCounts() map[string]int {
	return map[string]int{"CRITICAL": 0, "HIGH": 0, "MEDIUM": 0, "LOW": 0, "UNKNOWN": 0}
}

func compactStrings(values []string) []string {
	if len(values) == 0 {
		return values
	}
	compacted := values[:1]
	for _, value := range values[1:] {
		if value != compacted[len(compacted)-1] {
			compacted = append(compacted, value)
		}
	}
	return compacted
}

func sha256Hex(content []byte) string {
	digest := sha256.Sum256(content)
	return hex.EncodeToString(digest[:])
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra json.RawMessage
	if err := decoder.Decode(&extra); errors.Is(err, io.EOF) {
		return nil
	} else if err != nil {
		return fmt.Errorf("%w: trailing JSON: %w", ErrMalformedTrivyReport, err)
	}
	return fmt.Errorf("%w: trailing JSON value", ErrMalformedTrivyReport)
}
