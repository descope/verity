package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArchiveMetricsRetriesFromFreshRemoteWhenAnotherWriterAdvancesBranch(t *testing.T) {
	// Given: an archive clone and a competing writer sharing a bare remote.
	realGit, err := exec.LookPath("git")
	require.NoError(t, err)
	tmp := t.TempDir()
	remote := filepath.Join(tmp, "remote.git")
	seed := filepath.Join(tmp, "seed")
	archive := filepath.Join(tmp, "archive")
	competitor := filepath.Join(tmp, "competitor")

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
	runGit(t, realGit, "", "clone", "--branch", "main", remote, archive)
	runGit(t, realGit, "", "clone", "--branch", "_metrics", remote, competitor)
	configureGit(t, realGit, archive)
	configureGit(t, realGit, competitor)

	downloaded := filepath.Join(tmp, "downloaded")
	writeFile(t, filepath.Join(downloaded, "metrics-alpha.json"), `{"image":"alpha"}`+"\n")
	wrapperDir := filepath.Join(tmp, "bin")
	require.NoError(t, os.MkdirAll(wrapperDir, 0o755))
	marker := filepath.Join(tmp, "advanced")
	wrapper := `#!/usr/bin/env bash
set -euo pipefail
if [ "${1:-}" = "push" ] && [ ! -e "$ADVANCE_MARKER" ]; then
  touch "$ADVANCE_MARKER"
  "$REAL_GIT" -C "$COMPETITOR" fetch origin _metrics
  "$REAL_GIT" -C "$COMPETITOR" switch -C _metrics origin/_metrics
  mkdir -p "$COMPETITOR/_metrics/runs/2026-07-17/competing"
  printf '{"image":"competing"}\n' > "$COMPETITOR/_metrics/runs/2026-07-17/competing/competing.json"
  "$REAL_GIT" -C "$COMPETITOR" add _metrics
  "$REAL_GIT" -C "$COMPETITOR" commit -m 'competing metrics'
  "$REAL_GIT" -C "$COMPETITOR" push origin HEAD:refs/heads/_metrics
fi
exec "$REAL_GIT" "$@"
`
	writeExecutable(t, filepath.Join(wrapperDir, "git"), wrapper)

	// When: the archival writer's first push races with the competing commit.
	archiveScript, err := filepath.Abs(filepath.Join("..", ".github", "scripts", "archive-metrics.sh"))
	require.NoError(t, err)
	cmd := exec.CommandContext(t.Context(), "bash", archiveScript, downloaded, "123", "1", "2026-07-17T06:00:00Z")
	cmd.Dir = archive
	cmd.Env = append(
		os.Environ(),
		"PATH="+wrapperDir+":"+os.Getenv("PATH"),
		"REAL_GIT="+realGit,
		"COMPETITOR="+competitor,
		"ADVANCE_MARKER="+marker,
		"METRICS_ARCHIVE_ATTEMPTS=3",
		"METRICS_ARCHIVE_RETRY_DELAY_SECONDS=0",
	)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, string(output))

	// Then: retry two preserves both writers and never creates local `_metrics`.
	verify := filepath.Join(tmp, "verify")
	runGit(t, realGit, "", "clone", "--branch", "_metrics", remote, verify)
	assert.FileExists(t, filepath.Join(verify, "_metrics", "runs", "2026-07-17", "competing", "competing.json"))
	assert.FileExists(t, filepath.Join(verify, "_metrics", "runs", "2026-07-17", "123-attempt-1", "alpha.json"))
	branches := runGit(t, realGit, archive, "branch", "--list", "_metrics")
	assert.Empty(t, strings.TrimSpace(branches))
	assert.Contains(t, string(output), "Pushed on attempt 2")
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
  "status":"failure","failure_stage":"trivy","run_id":"42"
}`+"\n")

	// When: the failed reusable matrix is aggregated.
	cmd := exec.CommandContext(t.Context(), "bash", filepath.Join("..", ".github", "scripts", "aggregate-integer-results.sh"), expected, results, "failure", "verity-org/verity", "42")
	output, err := cmd.CombinedOutput()
	require.Error(t, err)

	// Then: every failed or unreported entry has identity, stage, run URL, and artifact reference.
	text := string(output)
	assert.Contains(t, text, "alpha:1.2.3-default")
	assert.Contains(t, text, "stage=trivy")
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
	cmd := exec.CommandContext(t.Context(), "bash", filepath.Join("..", ".github", "scripts", "aggregate-integer-results.sh"), expected, results, "skipped", "verity-org/verity", "42")
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
	cmd := exec.CommandContext(t.Context(), "bash", filepath.Join("..", ".github", "scripts", "aggregate-integer-results.sh"), expected, results, "skipped", "verity-org/verity", "42")
	output, err := cmd.CombinedOutput()

	// Then: the intentional no-op remains successful.
	require.NoError(t, err, string(output))
	assert.Contains(t, string(output), "No Integer child builds were dispatched.")
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
