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
