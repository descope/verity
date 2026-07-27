package repositoryops_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ops "github.com/verity-org/verity/internal/ci/repositoryops"
)

const trivyFixture = `{
  "Results": [
    {"Type":"gobinary","Vulnerabilities":[{"VulnerabilityID":"CVE-1"},{"VulnerabilityID":"CVE-2"}]},
    {"Type":"alpine","Vulnerabilities":[{"VulnerabilityID":"CVE-3"}]},
    {"Type":"python-pkg","Vulnerabilities":null}
  ]
}`

func TestScanService_Before_countsGoAndNonGoVulnerabilities(t *testing.T) {
	// Given
	reportPath := filepath.Join(t.TempDir(), "before.json")
	request, err := ops.NewScanBeforeRequest(ops.ScanBeforeInput{Source: "docker.io/library/alpine:3.22", ReportPath: reportPath})
	require.NoError(t, err)
	runner := reportWritingRunner(t, trivyFixture)

	// When
	result, err := (ops.ScanService{Commands: runner}).Before(context.Background(), request)

	// Then
	require.NoError(t, err)
	assert.Equal(t, ops.VulnerabilityCounts{Total: 3, Go: 2, NonGo: 1}, result.Counts)
	assert.False(t, result.SkipPatch)
	assert.Equal(t, []string{"image", "--severity", "CRITICAL,HIGH,MEDIUM,LOW", "--scanners", "vuln", "--format", "json", "--output", reportPath, "docker.io/library/alpine:3.22"}, runner.calls[0].Args)
}

func TestScanService_Verify_rejectsFixableNonGoVulnerability(t *testing.T) {
	// Given
	reportPath := filepath.Join(t.TempDir(), "after.json")
	request, err := ops.NewVerifyPatchedRequest(ops.VerifyPatchedInput{
		Image:      "ghcr.io/verity/alpine:3.22",
		ImageLabel: "alpine 3.22",
		ReportPath: reportPath,
		Before:     ops.VulnerabilityCounts{Total: 4, Go: 2, NonGo: 2},
	})
	require.NoError(t, err)
	runner := reportWritingRunner(t, trivyFixture)

	// When
	result, err := (ops.ScanService{Commands: runner}).Verify(context.Background(), request)

	// Then
	require.ErrorIs(t, err, ops.ErrFixableNonGoVulnerabilities)
	assert.Equal(t, 1, result.After.NonGo)
	assert.Contains(t, runner.calls[0].Args, "--ignore-unfixed")
}

func TestScanService_Before_rejectsMalformedTrivyReport(t *testing.T) {
	// Given
	reportPath := filepath.Join(t.TempDir(), "before.json")
	request, err := ops.NewScanBeforeRequest(ops.ScanBeforeInput{Source: "docker.io/library/alpine:3.22", ReportPath: reportPath})
	require.NoError(t, err)
	runner := reportWritingRunner(t, `{"Results":[`)

	// When
	_, err = (ops.ScanService{Commands: runner}).Before(context.Background(), request)

	// Then
	require.Error(t, err)
	assert.ErrorIs(t, err, ops.ErrMalformedTrivyReport)
}

func TestScanService_Before_rejectsReportWithoutResults(t *testing.T) {
	// Given
	reportPath := filepath.Join(t.TempDir(), "before.json")
	request, err := ops.NewScanBeforeRequest(ops.ScanBeforeInput{Source: "docker.io/library/alpine:3.22", ReportPath: reportPath})
	require.NoError(t, err)
	runner := reportWritingRunner(t, `{"SchemaVersion":2}`)

	// When
	_, err = (ops.ScanService{Commands: runner}).Before(context.Background(), request)

	// Then
	require.ErrorIs(t, err, ops.ErrMalformedTrivyReport)
}

func TestScanService_Before_rejectsAmbiguousTrivyJSON(t *testing.T) {
	tests := []struct {
		name   string
		report string
	}{
		{
			name:   "duplicate Results can hide vulnerabilities",
			report: `{"Results":[{"Type":"alpine","Vulnerabilities":[{"VulnerabilityID":"CVE-1"}]}],"Results":[]}`,
		},
		{name: "case variant Results", report: `{"results":[]}`},
		{name: "unknown top-level field", report: `{"Results":[],"Unexpected":true}`},
		{name: "trailing JSON value", report: `{"Results":[]} {}`},
		{
			name:   "duplicate Vulnerabilities can hide findings",
			report: `{"Results":[{"Type":"alpine","Vulnerabilities":[{"VulnerabilityID":"CVE-1"}],"Vulnerabilities":null}]}`,
		},
		{
			name:   "case variant Vulnerabilities",
			report: `{"Results":[{"Type":"alpine","vulnerabilities":[]}]}`,
		},
		{
			name:   "unknown result field",
			report: `{"Results":[{"Type":"alpine","Vulnerabilities":[],"Unexpected":true}]}`,
		},
		{
			name:   "case variant Type",
			report: `{"Results":[{"type":"gobinary","Vulnerabilities":[]}]}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given
			reportPath := filepath.Join(t.TempDir(), "before.json")
			request, err := ops.NewScanBeforeRequest(ops.ScanBeforeInput{
				Source: "docker.io/library/alpine:3.22", ReportPath: reportPath,
			})
			require.NoError(t, err)

			// When
			_, err = (ops.ScanService{Commands: reportWritingRunner(t, test.report)}).Before(t.Context(), request)

			// Then
			require.ErrorIs(t, err, ops.ErrMalformedTrivyReport)
		})
	}
}

func TestScanService_Before_acceptsPinnedTrivyReportSchema(t *testing.T) {
	// Given
	reportPath := filepath.Join(t.TempDir(), "before.json")
	request, err := ops.NewScanBeforeRequest(ops.ScanBeforeInput{
		Source: "docker.io/library/alpine:3.22", ReportPath: reportPath,
	})
	require.NoError(t, err)
	report := `{
  "SchemaVersion": 2,
  "Trivy": {"Version":"0.72.0"},
  "ReportID": "report-1",
  "CreatedAt": "2026-07-24T00:00:00Z",
  "ArtifactID": "artifact-1",
  "ArtifactName": "docker.io/library/alpine:3.22",
  "ArtifactType": "container_image",
  "Metadata": {},
  "Results": [{
    "Target":"alpine",
    "Class":"os-pkgs",
    "Type":"alpine",
    "Packages":[],
    "Vulnerabilities":[{"VulnerabilityID":"CVE-1","FutureField":"preserved"}],
    "MisconfSummary":{},
    "Misconfigurations":[],
    "Secrets":[],
    "Licenses":[],
    "CustomResources":[],
    "ExperimentalModifiedFindings":[]
  }]
}`

	// When
	result, err := (ops.ScanService{Commands: reportWritingRunner(t, report)}).Before(t.Context(), request)

	// Then
	require.NoError(t, err)
	assert.Equal(t, ops.VulnerabilityCounts{Total: 1, NonGo: 1}, result.Counts)
}

func TestScanService_Verify_rejectsStaleReportWhenTrivyWritesNothing(t *testing.T) {
	// Given
	reportPath := filepath.Join(t.TempDir(), "after.json")
	require.NoError(t, os.WriteFile(reportPath, []byte(`{"Results":[]}`), 0o600))
	request, err := ops.NewVerifyPatchedRequest(ops.VerifyPatchedInput{
		Image:      "ghcr.io/verity/alpine:3.22",
		ImageLabel: "alpine 3.22",
		ReportPath: reportPath,
		Before:     ops.VulnerabilityCounts{},
	})
	require.NoError(t, err)
	runner := &fakeCommandRunner{responses: []ops.CommandResult{{}}}

	// When
	_, err = (ops.ScanService{Commands: runner}).Verify(context.Background(), request)

	// Then
	require.ErrorIs(t, err, ops.ErrMalformedTrivyReport)
}

func reportWritingRunner(t *testing.T, report string) *fakeCommandRunner {
	t.Helper()
	return &fakeCommandRunner{run: func(_ context.Context, command ops.Command, _ int) (ops.CommandResult, error) {
		for index, argument := range command.Args {
			if argument == "--output" && index+1 < len(command.Args) {
				require.NoError(t, os.WriteFile(command.Args[index+1], []byte(report), 0o600))
				return ops.CommandResult{}, nil
			}
		}
		return ops.CommandResult{}, assert.AnError
	}}
}
