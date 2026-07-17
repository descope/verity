package scripts_test

import (
	"bufio"
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArchiveMetricsRetriesFromFreshRemoteWhenAnotherWriterAdvancesBranch(t *testing.T) {
	// Given: two archive producers sharing a bare remote.
	realGit, err := exec.LookPath("git")
	require.NoError(t, err)
	tmp := t.TempDir()
	remote := filepath.Join(tmp, "remote.git")
	seed := filepath.Join(tmp, "seed")
	archiveAlpha := filepath.Join(tmp, "archive-alpha")
	archiveBeta := filepath.Join(tmp, "archive-beta")

	runGit(t, realGit, "", "init", "--bare", remote)
	runGit(t, realGit, "", "init", "-b", "main", seed)
	configureGit(t, realGit, seed)
	writeFile(t, filepath.Join(seed, "README.md"), "seed\n")
	runGit(t, realGit, seed, "add", "README.md")
	runGit(t, realGit, seed, "commit", "-m", "seed main")
	runGit(t, realGit, seed, "remote", "add", "origin", remote)
	runGit(t, realGit, seed, "push", "origin", "main")
	runGit(t, realGit, seed, "switch", "--orphan", "_metrics")
	runGit(t, realGit, seed, "rm", "-rf", "--ignore-unmatch", "README.md")
	writeFile(t, filepath.Join(seed, "_metrics", "runs", "2026-07-16", "seed", "seed.json"), "{}\n")
	runGit(t, realGit, seed, "add", "_metrics")
	runGit(t, realGit, seed, "commit", "-m", "seed metrics")
	runGit(t, realGit, seed, "push", "origin", "_metrics")
	runGit(t, realGit, "", "clone", "--branch", "main", remote, archiveAlpha)
	runGit(t, realGit, "", "clone", "--branch", "main", remote, archiveBeta)
	configureGit(t, realGit, archiveAlpha)
	configureGit(t, realGit, archiveBeta)

	downloadedAlpha := filepath.Join(tmp, "downloaded-alpha")
	downloadedBeta := filepath.Join(tmp, "downloaded-beta")
	writeFile(t, filepath.Join(downloadedAlpha, "metrics-alpha.json"), `{"image":"alpha"}`+"\n")
	writeFile(t, filepath.Join(downloadedBeta, "metrics-beta.json"), `{"image":"beta"}`+"\n")
	wrapperDir := filepath.Join(tmp, "bin")
	require.NoError(t, os.MkdirAll(wrapperDir, 0o755))
	readyFIFO := filepath.Join(tmp, "alpha-push-ready")
	releaseFIFO := filepath.Join(tmp, "release-alpha-push")
	mkfifo, err := exec.LookPath("mkfifo")
	require.NoError(t, err)
	for _, fifo := range []string{readyFIFO, releaseFIFO} {
		cmd := exec.CommandContext(t.Context(), mkfifo, fifo)
		output, commandErr := cmd.CombinedOutput()
		require.NoError(t, commandErr, string(output))
	}
	ready, err := os.OpenFile(readyFIFO, os.O_RDWR, 0)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, ready.Close()) })
	release, err := os.OpenFile(releaseFIFO, os.O_RDWR, 0)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, release.Close()) })

	barrierUsed := filepath.Join(tmp, "barrier-used")
	wrapper := `#!/usr/bin/env bash
set -euo pipefail
if [ "${1:-}" = "push" ] && [ ! -e "$BARRIER_USED" ]; then
  touch "$BARRIER_USED"
  printf 'ready\n' > "$READY_FIFO"
  IFS= read -r _ < "$RELEASE_FIFO"
fi
exec "$REAL_GIT" "$@"
`
	writeExecutable(t, filepath.Join(wrapperDir, "git"), wrapper)

	// When: alpha reaches push first, beta advances the branch, then alpha retries.
	archiveScript, err := filepath.Abs(filepath.Join("..", ".github", "scripts", "archive-metrics.sh"))
	require.NoError(t, err)
	alpha := exec.CommandContext(t.Context(), "bash", archiveScript, downloadedAlpha, "123", "1", "2026-07-17T06:00:00Z")
	alpha.Dir = archiveAlpha
	alpha.Env = append(
		os.Environ(),
		"PATH="+wrapperDir+":"+os.Getenv("PATH"),
		"REAL_GIT="+realGit,
		"READY_FIFO="+readyFIFO,
		"RELEASE_FIFO="+releaseFIFO,
		"BARRIER_USED="+barrierUsed,
		"METRICS_ARCHIVE_ATTEMPTS=3",
		"METRICS_ARCHIVE_RETRY_DELAY_SECONDS=0",
		"METRICS_ARCHIVE_RETRY_JITTER_SECONDS=0",
	)
	var alphaOutput bytes.Buffer
	alpha.Stdout = &alphaOutput
	alpha.Stderr = &alphaOutput
	require.NoError(t, alpha.Start())
	readySignal, err := bufio.NewReader(ready).ReadString('\n')
	require.NoError(t, err)
	require.Equal(t, "ready\n", readySignal)

	beta := exec.CommandContext(t.Context(), "bash", archiveScript, downloadedBeta, "456", "1", "2026-07-17T06:01:00Z")
	beta.Dir = archiveBeta
	beta.Env = append(
		os.Environ(),
		"METRICS_ARCHIVE_ATTEMPTS=3",
		"METRICS_ARCHIVE_RETRY_DELAY_SECONDS=0",
		"METRICS_ARCHIVE_RETRY_JITTER_SECONDS=0",
	)
	betaOutput, err := beta.CombinedOutput()
	require.NoError(t, err, string(betaOutput))
	_, err = release.WriteString("release\n")
	require.NoError(t, err)
	require.NoError(t, alpha.Wait(), alphaOutput.String())

	// Then: both real producers survive and neither creates a local `_metrics` branch.
	verify := filepath.Join(tmp, "verify")
	runGit(t, realGit, "", "clone", "--branch", "_metrics", remote, verify)
	assert.FileExists(t, filepath.Join(verify, "_metrics", "runs", "2026-07-17", "123-attempt-1", "alpha.json"))
	assert.FileExists(t, filepath.Join(verify, "_metrics", "runs", "2026-07-17", "456-attempt-1", "beta.json"))
	for _, archive := range []string{archiveAlpha, archiveBeta} {
		branches := runGit(t, realGit, archive, "branch", "--list", "_metrics")
		assert.Empty(t, strings.TrimSpace(branches))
	}
	assert.Contains(t, alphaOutput.String(), "Pushed on attempt 2")
	assert.Contains(t, string(betaOutput), "Pushed on attempt 1")
}

func TestAggregateIntegerResultsNamesFailedAndMissingMatrixEntries(t *testing.T) {
	// Given: one failed child report and one planned child with no report artifact.
	tmp := t.TempDir()
	expected := filepath.Join(tmp, "expected.json")
	results := filepath.Join(tmp, "results")
	writeFile(t, expected, `[
  {"name":"alpha","version":"1.2.3","type":"default"},
  {"name":"beta/tools","version":"4.5.6","type":"fips"}
]`+"\n")
	writeFile(t, filepath.Join(results, "integer-build-result-alpha", "report.json"), `{
  "image":"alpha","version":"1.2.3","type":"default",
	  "status":"failure","failure_stage":"trivy","run_id":"42",
	  "batch_id":"42-1","shard":1
}`+"\n")

	// When: the failed reusable matrix is aggregated.
	cmd := exec.CommandContext(t.Context(), "bash", filepath.Join("..", ".github", "scripts", "aggregate-integer-results.sh"), expected, results, "failure", "verity-org/verity", "42", "42-1")
	output, err := cmd.CombinedOutput()
	require.Error(t, err)

	// Then: every failed or unreported entry has identity, stage, run URL, and artifact reference.
	text := string(output)
	assert.Contains(t, text, "alpha:1.2.3-default")
	assert.Contains(t, text, "stage=trivy")
	assert.Contains(t, text, "shard=1")
	assert.Contains(t, text, "batch=42-1")
	assert.Contains(t, text, "integer-build-result-alpha-1.2.3-default")
	assert.Contains(t, text, "beta/tools:4.5.6-fips")
	assert.Contains(t, text, "stage=matrix-dispatch-or-report")
	assert.Contains(t, text, "integer-build-result-beta-tools-4.5.6-fips")
	assert.Contains(t, text, "https://github.com/verity-org/verity/actions/runs/42")
}

func TestAggregateIntegerResultsFailsClosedWhenNonEmptyPlanIsSkipped(t *testing.T) {
	// Given: a planned matrix entry whose reusable workflow was skipped.
	tmp := t.TempDir()
	expected := filepath.Join(tmp, "expected.json")
	results := filepath.Join(tmp, "results")
	writeFile(t, expected, `[{"name":"alpha","version":"1.2.3","type":"default"}]`+"\n")
	require.NoError(t, os.MkdirAll(results, 0o755))

	// When: aggregation receives the skipped matrix result.
	cmd := exec.CommandContext(t.Context(), "bash", filepath.Join("..", ".github", "scripts", "aggregate-integer-results.sh"), expected, results, "skipped", "verity-org/verity", "42", "42-1")
	output, err := cmd.CombinedOutput()

	// Then: the undispatched entry is reported instead of accepted as success.
	require.Error(t, err)
	assert.Contains(t, string(output), "alpha:1.2.3-default")
	assert.Contains(t, string(output), "stage=matrix-dispatch-or-report")
}

func TestAggregateIntegerResultsAcceptsSkippedEmptyPlan(t *testing.T) {
	// Given: discovery intentionally produced no Integer matrix entries.
	tmp := t.TempDir()
	expected := filepath.Join(tmp, "expected.json")
	results := filepath.Join(tmp, "results")
	writeFile(t, expected, "[]\n")
	require.NoError(t, os.MkdirAll(results, 0o755))

	// When: the empty matrix is reported as skipped.
	cmd := exec.CommandContext(t.Context(), "bash", filepath.Join("..", ".github", "scripts", "aggregate-integer-results.sh"), expected, results, "skipped", "verity-org/verity", "42", "42-1")
	output, err := cmd.CombinedOutput()

	// Then: the intentional no-op remains successful.
	require.NoError(t, err, string(output))
	assert.Contains(t, string(output), "No Integer child builds were dispatched.")
}

func TestAggregateIntegerResultsRejectsSuccessfulPartialDispatch(t *testing.T) {
	// Given: a successful shard result that omitted one planned entry.
	tmp := t.TempDir()
	expected := filepath.Join(tmp, "expected.json")
	results := filepath.Join(tmp, "results")
	writeFile(t, expected, `[
  {"name":"alpha","version":"1","type":"default"},
  {"name":"beta","version":"2","type":"default"}
]`+"\n")
	writeFile(t, filepath.Join(results, "integer-build-result-alpha", "report.json"), `{
  "image":"alpha","version":"1","type":"default","status":"success",
  "run_id":"42","batch_id":"42-1","shard":1
}`+"\n")

	// When
	cmd := exec.CommandContext(t.Context(), "bash", filepath.Join("..", ".github", "scripts", "aggregate-integer-results.sh"), expected, results, "success", "verity-org/verity", "42", "42-1")
	output, err := cmd.CombinedOutput()

	// Then
	require.Error(t, err)
	assert.Contains(t, string(output), "beta:2-default")
	assert.Contains(t, string(output), "stage=matrix-dispatch-or-report")
}

func TestAggregateIntegerResultsAcceptsCompleteSuccessfulShards(t *testing.T) {
	// Given: every planned entry has one current-batch successful report.
	tmp := t.TempDir()
	expected := filepath.Join(tmp, "expected.json")
	results := filepath.Join(tmp, "results")
	writeFile(t, expected, `[
  {"name":"alpha","version":"1","type":"default"},
  {"name":"beta","version":"2","type":"fips"}
]`+"\n")
	writeFile(t, filepath.Join(results, "alpha", "report.json"), `{
  "image":"alpha","version":"1","type":"default","status":"success",
  "run_id":"42","batch_id":"42-1","shard":1
}`+"\n")
	writeFile(t, filepath.Join(results, "beta", "report.json"), `{
  "image":"beta","version":"2","type":"fips","status":"success",
  "run_id":"42","batch_id":"42-1","shard":1
}`+"\n")

	// When
	cmd := exec.CommandContext(t.Context(), "bash", filepath.Join("..", ".github", "scripts", "aggregate-integer-results.sh"), expected, results, "success", "verity-org/verity", "42", "42-1")
	output, err := cmd.CombinedOutput()

	// Then
	require.NoError(t, err, string(output))
	assert.Contains(t, string(output), "All 2 planned Integer child builds succeeded across 1 shard(s).")
}

func configureGit(t *testing.T, gitPath, dir string) {
	t.Helper()
	runGit(t, gitPath, dir, "config", "user.name", "test")
	runGit(t, gitPath, dir, "config", "user.email", "test@example.com")
}

func runGit(t *testing.T, gitPath, dir string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), gitPath, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s: %s", strings.Join(args, " "), output)
	return string(output)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	writeFile(t, path, content)
	require.NoError(t, os.Chmod(path, 0o755))
}
