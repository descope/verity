package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

func runCoveragePRCLI(t *testing.T, arguments ...string) (stdoutText, stderrText string, err error) {
	t.Helper()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	root := &cli.Command{Writer: &stdout, ErrWriter: &stderr, Commands: []*cli.Command{newCIPrTestCommand()}}
	err = root.Run(t.Context(), append([]string{"verity", "pr-test"}, arguments...))
	return stdout.String(), stderr.String(), err
}

func parseCoveragePRGitHubOutput(t *testing.T, path string) map[string]string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	values := make(map[string]string)
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		name, value, found := strings.Cut(line, "=")
		require.True(t, found, line)
		values[name] = value
	}
	return values
}

func coveragePRAggregateArguments() []string {
	return []string{
		"aggregate",
		"--changes-result", "success", "--integer", "false", "--copa", "false",
		"--discover-result", "skipped", "--validate-result", "skipped",
		"--detect-integer-result", "skipped", "--integer-has-changes", "false",
		"--integer-smoke-result", "skipped", "--integer-build-result", "skipped",
		"--expected-integer-matrix", `{"include":[]}`,
		"--expected-integer-smoke-matrix", `{"include":[]}`,
		"--detect-copa-result", "skipped", "--copa-changed-result", "skipped", "--copa-regression-result", "skipped",
	}
}

func setCoveragePRFlag(arguments []string, name, value string) {
	for index := range arguments {
		if arguments[index] == "--"+name {
			arguments[index+1] = value
			return
		}
	}
}

func writeCoveragePRTool(t *testing.T, directory, mode string) string {
	t.Helper()
	path := filepath.Join(directory, strings.TrimPrefix(mode, "fake-"))
	script := fmt.Sprintf("#!/bin/sh\nexport %s=%s\nexec %s -test.run=^TestCoveragePRCommandHelperProcess$ -- \"$@\"\n", coveragePRCommandHelper, mode, strconv.Quote(os.Args[0]))
	require.NoError(t, os.WriteFile(path, []byte(script), 0o700))
	return path
}

func TestCIPrPlanIntegerCLI_emits_native_matrices_from_committed_change(t *testing.T) {
	// Given: a repository whose head changes one committed Integer definition.
	repoRoot := setupIntegerBatchGitRepository(t)
	baseSHA := runIntegerBatchGit(t, repoRoot, "rev-parse", "HEAD")
	writeCommandFixture(t, filepath.Join(repoRoot, "images", "alpha.yaml"), integerBatchImage("alpha", "changed"))
	runIntegerBatchGit(t, repoRoot, "add", "images/alpha.yaml")
	runIntegerBatchGit(t, repoRoot, "commit", "-q", "-m", "head")
	headSHA := runIntegerBatchGit(t, repoRoot, "rev-parse", "HEAD")
	githubOutput := filepath.Join(t.TempDir(), "github-output")

	// When: the public PR planner runs headlessly over the committed range.
	stdout, stderr, err := runCoveragePRCLI(
		t,
		"plan-integer",
		"--base-sha", baseSHA,
		"--head-sha", headSHA,
		"--repo-root", repoRoot,
		"--temp-dir", t.TempDir(),
		"--integer-config", filepath.Join(repoRoot, "integer.yaml"),
		"--images-dir", filepath.Join(repoRoot, "images"),
		"--apkindex-url=",
		"--github-output", githubOutput,
	)

	// Then: strict and expected matrices retain one target across both native legs.
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "1 strict builds and 0 smoke-only variants in 2 native legs")
	values := parseCoveragePRGitHubOutput(t, githubOutput)
	require.Equal(t, "true", values["has-changes"])
	require.Equal(t, "false", values["smoke-has-changes"])
	var matrix prIntegerBatchMatrix
	require.NoError(t, json.Unmarshal([]byte(values["matrix"]), &matrix))
	require.Len(t, matrix.Include, 2)
	require.Equal(t, "amd64", matrix.Include[0].Architecture)
	require.Equal(t, "arm64", matrix.Include[1].Architecture)
	require.Equal(t, []prIntegerEntry{{Image: "alpha", Version: "latest", Type: "default"}}, matrix.Include[0].Entries)
	var expected prIntegerExpectedMatrix
	require.NoError(t, json.Unmarshal([]byte(values["expected-matrix"]), &expected))
	require.Equal(t, matrix.Include[0].Entries, expected.Include)
}

func TestStagePRIntegerBaseImage_rejects_path_traversal(t *testing.T) {
	// Given: a changed image path that escapes the images directory.
	request := prIntegerBaseImageRequest{ImagesDir: t.TempDir(), Path: "images/../sentinel.yaml"}

	// When: base-image staging validates the path.
	err := stagePRIntegerBaseImage(t.Context(), request)

	// Then: traversal is rejected before any Git access.
	require.ErrorIs(t, err, errPRCommandFailed)
	require.ErrorContains(t, err, "unsafe changed image path")
}

func TestCIPrAggregateCLI_accepts_inactive_and_active_no_change_scopes(t *testing.T) {
	tests := []struct {
		name      string
		configure func([]string)
		want      string
	}{
		{name: "inactive scopes", configure: func([]string) {}, want: "integer: false\ncopa: false"},
		{name: "active scopes without Integer changes", configure: func(arguments []string) {
			for name, value := range map[string]string{
				"integer": "true", "copa": "true", "discover-result": "success",
				"validate-result": "success", "detect-integer-result": "success",
				"detect-copa-result": "success", "copa-regression-result": "success",
			} {
				setCoveragePRFlag(arguments, name, value)
			}
		}, want: "integer: true\ncopa: true"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given: exact GitHub job results for the selected PR scopes.
			arguments := coveragePRAggregateArguments()
			test.configure(arguments)

			// When: final aggregation runs through the public command.
			stdout, stderr, err := runCoveragePRCLI(t, arguments...)

			// Then: aggregation succeeds and reports the selected scopes.
			require.NoError(t, err)
			require.Empty(t, stderr)
			require.Contains(t, stdout, test.want)
		})
	}
}

func TestCIPrAggregateCLI_rejects_invalid_boundaries(t *testing.T) {
	validMatrix := `{"include":[{"image":"demo","version":"1","type":"default"}]}`
	tests := []struct {
		name, want string
		values     map[string]string
	}{
		{name: "integer boolean", values: map[string]string{"integer": "yes"}, want: "integer must be true or false"},
		{name: "copa boolean", values: map[string]string{"copa": "yes"}, want: "copa must be true or false"},
		{name: "change boolean", values: map[string]string{"integer-has-changes": "yes"}, want: "integer-has-changes must be true or false"},
		{name: "required result", values: map[string]string{"changes-result": "failure"}, want: "changes did not succeed"},
		{name: "active Integer validate", values: map[string]string{"integer": "true", "validate-result": "failure"}, want: "validate did not succeed"},
		{name: "active Integer detection", values: map[string]string{"integer": "true", "validate-result": "success", "detect-integer-result": "failure"}, want: "detect-changed-images did not succeed"},
		{name: "active Integer smoke", values: map[string]string{"integer": "true", "validate-result": "success", "detect-integer-result": "success", "integer-smoke-result": "failure"}, want: "integer-smoke-test did not succeed or skip cleanly"},
		{name: "inactive Integer job", values: map[string]string{"validate-result": "failure"}, want: "validate did not succeed or skip cleanly"},
		{name: "active Copa discovery", values: map[string]string{"copa": "true", "discover-result": "failure"}, want: "discover did not succeed"},
		{name: "active Copa detection", values: map[string]string{"copa": "true", "discover-result": "success", "detect-copa-result": "failure"}, want: "detect-copa-changes did not succeed"},
		{name: "active Copa changed", values: map[string]string{"copa": "true", "discover-result": "success", "detect-copa-result": "success", "copa-changed-result": "failure"}, want: "copa-patching-changed did not succeed or skip cleanly"},
		{name: "inactive Copa job", values: map[string]string{"discover-result": "failure"}, want: "discover did not succeed or skip cleanly"},
		{name: "trailing build matrix", values: map[string]string{"integer": "true", "validate-result": "success", "detect-integer-result": "success", "integer-has-changes": "true", "expected-integer-matrix": `{"include":[]} {}`}, want: "invalid expected Integer build matrix"},
		{name: "empty changed build", values: map[string]string{"integer": "true", "validate-result": "success", "detect-integer-result": "success", "integer-has-changes": "true"}, want: "expected Integer build matrix must not be empty"},
		{name: "non-skipped empty smoke", values: map[string]string{"integer": "true", "validate-result": "success", "detect-integer-result": "success", "integer-has-changes": "true", "integer-smoke-result": "success", "expected-integer-matrix": validMatrix}, want: "integer-smoke-test must be skipped"},
		{name: "required smoke result", values: map[string]string{"integer": "true", "validate-result": "success", "detect-integer-result": "success", "integer-has-changes": "true", "expected-integer-matrix": validMatrix, "expected-integer-smoke-matrix": validMatrix}, want: "integer-smoke-test did not succeed"},
		{name: "malformed smoke matrix", values: map[string]string{"integer": "true", "validate-result": "success", "detect-integer-result": "success", "integer-has-changes": "true", "expected-integer-matrix": validMatrix, "expected-integer-smoke-matrix": "{"}, want: "invalid expected Integer smoke matrix"},
		{name: "unsafe marker identity", values: map[string]string{"integer": "true", "validate-result": "success", "detect-integer-result": "success", "integer-has-changes": "true", "integer-build-result": "success", "expected-integer-matrix": `{"include":[{"image":"../demo","version":"1","type":"default"}]}`}, want: "invalid Integer build marker identity"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given: one malformed aggregate boundary value.
			arguments := coveragePRAggregateArguments()
			for name, value := range test.values {
				setCoveragePRFlag(arguments, name, value)
			}

			// When: the aggregate command parses and evaluates the inputs.
			_, _, err := runCoveragePRCLI(t, arguments...)

			// Then: the boundary fails closed with the exact field name.
			require.ErrorIs(t, err, errPRCommandFailed)
			require.ErrorContains(t, err, test.want)
		})
	}
}
