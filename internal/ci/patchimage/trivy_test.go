package patchimage

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/ci/workflowops/retry"
)

const trivyReportFixture = `{
  "Results": [
    {"Vulnerabilities":[
      {"VulnerabilityID":"CVE-2","Severity":"HIGH","PkgName":"zlib"},
      {"VulnerabilityID":"CVE-1","Severity":"CRITICAL","PkgName":"openssl"},
      {"VulnerabilityID":"CVE-1","Severity":"HIGH","PkgName":"openssl"}
    ]},
    {"Vulnerabilities":null}
  ]
}`

func TestParseTrivyReport_preservesCountSeverityAndStableFingerprints(t *testing.T) {
	// Given / When
	summary, err := ParseTrivyReport([]byte(trivyReportFixture))

	// Then
	require.NoError(t, err)
	assert.Equal(t, 3, summary.Count)
	assert.Equal(t, map[string]int{
		"CRITICAL": 1,
		"HIGH":     2,
		"LOW":      0,
		"MEDIUM":   0,
		"UNKNOWN":  0,
	}, summary.BySeverity)
	assert.Equal(t, "c91c5c9615fa6bc804444d1f1e558eeb2ca9c3fc26994ae29909174654b2af4a", summary.VulnerabilityFingerprint())
	assert.Equal(t, "7f8b47a2a03616bae9044b82532f092dfabe85709a97e1af1cd33acbcdd29689", summary.PackageFingerprint())
}

func TestScanService_Scan_writesReportAndReturnsCount(t *testing.T) {
	// Given
	reportPath := filepath.Join(t.TempDir(), "pre.json")
	runner := &fakeRunner{run: func(_ context.Context, command *retry.Command) (retry.Result, error) {
		assert.Equal(t, "trivy", command.Name)
		assert.Contains(t, command.Args, "--output")
		require.NoError(t, os.WriteFile(reportPath, []byte(trivyReportFixture), 0o600))
		return retry.Result{}, nil
	}}

	// When
	result, err := (ScanService{Runner: runner}).Scan(t.Context(), ScanRequest{
		Image: "registry.example/image:v1", ReportPath: reportPath,
	})

	// Then
	require.NoError(t, err)
	assert.Equal(t, 3, result.Count)
}

func TestScanService_CheckExisting_failsClosedWhenTrivyFails(t *testing.T) {
	// Given
	runner := &fakeRunner{results: []runnerResult{
		{result: retry.Result{Stdout: []byte("sha256:abc\n")}},
		{err: assert.AnError},
	}}

	// When
	result, err := (ScanService{Runner: runner}).CheckExisting(t.Context(), ExistingImageRequest{
		Image: "ghcr.io/verity/image:v1", ReportPath: filepath.Join(t.TempDir(), "existing.json"),
	})

	// Then
	require.NoError(t, err)
	assert.True(t, result.NeedsPatch)
}

type runnerResult struct {
	result retry.Result
	err    error
}

type fakeRunner struct {
	results []runnerResult
	run     func(context.Context, *retry.Command) (retry.Result, error)
	calls   []retry.Command
}

func (runner *fakeRunner) Run(ctx context.Context, command *retry.Command) (retry.Result, error) {
	runner.calls = append(runner.calls, *command)
	if runner.run != nil {
		return runner.run(ctx, command)
	}
	if len(runner.results) == 0 {
		return retry.Result{}, assert.AnError
	}
	result := runner.results[0]
	runner.results = runner.results[1:]
	return result.result, result.err
}
