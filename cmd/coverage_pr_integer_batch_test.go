package cmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

func TestCIPrIntegerBatchCLI_executes_build_with_headless_tools(t *testing.T) {
	// Given: generated tool shims that implement the external command contracts.
	repoRoot := t.TempDir()
	toolDirectory := t.TempDir()
	verityPath := writeCoveragePRTool(t, toolDirectory, "fake-verity")
	writeCoveragePRTool(t, toolDirectory, "fake-docker")
	writeCoveragePRTool(t, toolDirectory, "fake-trivy")
	t.Setenv("PATH", toolDirectory+string(os.PathListSeparator)+os.Getenv("PATH"))
	securityDir := filepath.Join(repoRoot, "security")
	reportsDir := filepath.Join(repoRoot, "reports")

	// When: the production integer-batch action executes a generic strict build.
	stdout, stderr, err := runCoveragePRCLI(
		t,
		"integer-batch", "--kind", "build",
		"--entries", `[{"image":"demo","version":"1","type":"default"}]`,
		"--arch", "amd64", "--package-arch", "x86_64", "--repo-root", repoRoot,
		"--runner-temp", t.TempDir(), "--security-dir", securityDir, "--reports-dir", reportsDir,
		"--verity", verityPath,
	)

	// Then: orchestration writes the zero-vulnerability report and native marker.
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "demo:1-default amd64: Total vulnerabilities: 0")
	require.FileExists(t, filepath.Join(reportsDir, "demo-1-default", "amd64.json"))
	require.FileExists(t, filepath.Join(securityDir, "build-demo-1-default-amd64.passed"))
}

func TestWaitPRInteger_observes_timer_and_cancellation(t *testing.T) {
	t.Run("elapsed timer", func(t *testing.T) {
		require.NoError(t, waitPRInteger(t.Context(), 0))
	})
	t.Run("canceled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		require.ErrorIs(t, waitPRInteger(ctx, time.Hour), context.Canceled)
	})
}

func TestCIPrIntegerBatchCLI_parses_valid_request_headlessly(t *testing.T) {
	// Given: the production CLI flags with execution replaced by request capture.
	command := newCIPrIntegerBatchCommand()
	var captured prIntegerBatchRequest
	command.Action = func(_ context.Context, command *cli.Command) error {
		request, err := newPRIntegerBatchRequest(command)
		captured = request
		return err
	}
	root := &cli.Command{Commands: []*cli.Command{command}}
	repoRoot := filepath.Join(t.TempDir(), "repo", "..", "repo")

	// When: a valid arm64 build request is parsed without runtime side effects.
	err := root.Run(t.Context(), []string{
		"verity", "integer-batch", "--kind", "build",
		"--entries", `[{"image":"demo/path","version":"1","type":"default"}]`,
		"--arch", "arm64", "--package-arch", "aarch64", "--repo-root", repoRoot,
		"--runner-temp", t.TempDir(), "--security-dir", t.TempDir(), "--reports-dir", t.TempDir(),
		"--verity", "/sentinel/verity",
	})

	// Then: typed fields and normalized paths are ready for orchestration.
	require.NoError(t, err)
	require.Equal(t, prIntegerBatchBuild, captured.Kind)
	require.Equal(t, "arm64", captured.Architecture)
	require.Equal(t, "aarch64", captured.PackageArchitecture)
	require.Equal(t, filepath.Clean(repoRoot), captured.RepoRoot)
	require.Equal(t, "/sentinel/verity", captured.VerityPath)
	require.Equal(t, []prIntegerEntry{{Image: "demo/path", Version: "1", Type: "default"}}, captured.Entries)
}

func TestCIPrIntegerBatchCLI_rejects_invalid_requests_before_execution(t *testing.T) {
	validEntry := `[{"image":"demo","version":"1","type":"default"}]`
	tests := []struct {
		name, kind, entries, architecture, packageArchitecture, want string
	}{
		{name: "kind", kind: "deploy", entries: validEntry, architecture: "amd64", packageArchitecture: "x86_64", want: "invalid Integer batch kind"},
		{name: "architecture pair", kind: "smoke", entries: validEntry, architecture: "amd64", packageArchitecture: "aarch64", want: "mismatched Integer architectures"},
		{name: "empty entries", kind: "smoke", entries: `[]`, architecture: "amd64", packageArchitecture: "x86_64", want: "non-empty JSON array"},
		{name: "unknown field", kind: "smoke", entries: `[{"image":"demo","version":"1","type":"default","extra":true}]`, architecture: "amd64", packageArchitecture: "x86_64", want: "non-empty JSON array"},
		{name: "trailing JSON", kind: "smoke", entries: validEntry + `{}`, architecture: "amd64", packageArchitecture: "x86_64", want: "invalid Integer entries JSON"},
		{name: "incomplete entry", kind: "smoke", entries: `[{"image":"","version":"1","type":"default"}]`, architecture: "amd64", packageArchitecture: "x86_64", want: "incomplete Integer batch entry"},
		{name: "unsafe entry", kind: "smoke", entries: `[{"image":"../demo","version":"1","type":"default"}]`, architecture: "amd64", packageArchitecture: "x86_64", want: "unsafe Integer batch entry"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given: a complete CLI request with one invalid typed field.
			arguments := []string{
				"integer-batch", "--kind", test.kind, "--entries", test.entries,
				"--arch", test.architecture, "--package-arch", test.packageArchitecture,
			}

			// When: the production command validates before creating dependencies.
			_, _, err := runCoveragePRCLI(t, arguments...)

			// Then: execution stops at the typed request boundary.
			require.ErrorIs(t, err, errPRCommandFailed)
			require.ErrorContains(t, err, test.want)
		})
	}
}
