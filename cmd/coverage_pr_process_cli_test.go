package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAppendPRGitHubValues_appends_valid_values_and_rejects_injection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "github-output")
	require.NoError(t, appendPRGitHubValues(path, [][2]string{{"alpha", "one"}, {"beta", "two"}}))
	require.NoError(t, appendPRGitHubValues(path, [][2]string{{"gamma", "three"}}))

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "alpha=one\nbeta=two\ngamma=three\n", string(data))
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	for _, test := range []struct {
		name, path string
		values     [][2]string
	}{
		{name: "blank path", path: "", values: [][2]string{{"alpha", "one"}}},
		{name: "invalid key", path: filepath.Join(t.TempDir(), "key"), values: [][2]string{{"bad=key", "one"}}},
		{name: "invalid value", path: filepath.Join(t.TempDir(), "value"), values: [][2]string{{"alpha", "line\nbreak"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := appendPRGitHubValues(test.path, test.values)
			require.ErrorIs(t, err, errPRCommandFailed)
		})
	}
	require.ErrorContains(t, appendPRGitHubValues(t.TempDir(), [][2]string{{"alpha", "one"}}), "open GitHub output")
}

func TestCIPrScopeCLI_reads_explicit_changed_files_headlessly(t *testing.T) {
	changedFile := filepath.Join(t.TempDir(), "changed-files")
	require.NoError(t, os.WriteFile(changedFile, []byte(" docs/guide.md \nimages/demo.yaml\ninternal/patch/demo.go\n\n"), 0o600))
	githubOutput := filepath.Join(t.TempDir(), "github-output")

	stdout, _, err := runCoveragePRCLI(t, "scope", "--changed-files", changedFile, "--github-output", githubOutput)

	require.NoError(t, err)
	require.Contains(t, stdout, "integer=true copa=true changed-paths=3")
	values := parseCoveragePRGitHubOutput(t, githubOutput)
	require.Equal(t, "true", values[integerCommandName])
	require.Equal(t, "true", values["copa"])
}

func TestCIPrDiscoverCLI_runs_typed_process_headlessly(t *testing.T) {
	toolDirectory := t.TempDir()
	verityPath := writeCoveragePRTool(t, toolDirectory, "fake-verity")
	output := filepath.Join(t.TempDir(), "images.json")

	stdout, stderr, err := runCoveragePRCLI(
		t,
		"discover", "--verity", verityPath, "--repo-root", t.TempDir(),
		"--target-registry", "registry.example", "--output", output,
	)

	require.NoError(t, err)
	require.Empty(t, stderr)
	data, readErr := os.ReadFile(output)
	require.NoError(t, readErr)
	require.Equal(t, `[{"name":"sentinel","source":"registry.example/sentinel:1"}]`, strings.TrimSpace(string(data)))
	require.Contains(t, stdout, "Discovered 1 image+tag combos")
	require.Contains(t, stdout, "sentinel: registry.example/sentinel:1")
}
