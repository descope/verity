package scripts_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func melangeBuildScriptPath(t *testing.T) string {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	require.True(t, ok)

	return filepath.Join(filepath.Dir(currentFile), "..", ".github", "scripts", "melange-build.sh")
}

func writeTempFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func runMelangeBuild(t *testing.T, repoRoot string, env map[string]string) (string, error) {
	t.Helper()

	cmd := exec.CommandContext(context.Background(), "bash", melangeBuildScriptPath(t))
	cmd.Dir = repoRoot
	cmd.Env = os.Environ()
	for key, value := range env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}

	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestMelangeBuildRejectsUnsafeBespokeValue(t *testing.T) {
	repoRoot := t.TempDir()

	output, err := runMelangeBuild(t, repoRoot, map[string]string{
		"BESPOKE": "../evil.yaml",
	})

	require.Error(t, err)
	assert.Contains(t, output, "BESPOKE contains unsafe characters")
}

func TestMelangeBuildFailsWhenBespokeFileMissing(t *testing.T) {
	repoRoot := t.TempDir()
	writeTempFile(t, filepath.Join(repoRoot, "packages", "upstream.lock.json"), `{"wolfi_commit":"abc123"}`)

	output, err := runMelangeBuild(t, repoRoot, map[string]string{
		"BESPOKE": "custom.yaml",
	})

	require.Error(t, err)
	assert.Contains(t, output, "Bespoke build file not found: packages/bespoke/custom.yaml")
}

func TestMelangeBuildFailsWhenBespokeWolfiCommitMissing(t *testing.T) {
	repoRoot := t.TempDir()
	writeTempFile(t, filepath.Join(repoRoot, "packages", "bespoke", "custom.yaml"), "package:\n  name: test\n")
	writeTempFile(t, filepath.Join(repoRoot, "packages", "upstream.lock.json"), `{"wolfi_commit":null}`)

	output, err := runMelangeBuild(t, repoRoot, map[string]string{
		"BESPOKE": "custom.yaml",
	})

	require.Error(t, err)
	assert.Contains(t, output, "wolfi_commit missing or null in packages/upstream.lock.json")
}
