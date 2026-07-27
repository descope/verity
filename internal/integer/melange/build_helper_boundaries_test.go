package melange

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_ExecRunner_forwards_directory_environment_and_output(t *testing.T) {
	// Given
	executable, err := os.Executable()
	require.NoError(t, err)
	workingDir := t.TempDir()
	command := &Command{
		Name: executable,
		Args: []string{"-test.run=^Test_ExecRunner_helper_process$"},
		Dir:  workingDir,
		Env: []string{
			"MELANGE_EXEC_HELPER=1",
			"MELANGE_EXEC_VALUE=sentinel",
		},
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	// When
	err = (ExecRunner{}).Run(context.Background(), command, &stdout, &stderr)

	// Then
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "value=sentinel")
	assert.Contains(t, stdout.String(), "cwd="+workingDir)
	assert.Contains(t, stderr.String(), "sentinel stderr")
}

func Test_ExecRunner_returns_subprocess_exit_error(t *testing.T) {
	// Given
	executable, err := os.Executable()
	require.NoError(t, err)
	command := &Command{
		Name: executable,
		Args: []string{"-test.run=^Test_ExecRunner_helper_process$"},
		Env:  []string{"MELANGE_EXEC_HELPER=1", "MELANGE_EXEC_FAIL=1"},
	}

	// When
	err = (ExecRunner{}).Run(context.Background(), command, io.Discard, io.Discard)

	// Then
	require.Error(t, err)
}

func Test_ExecRunner_helper_process(t *testing.T) {
	if os.Getenv("MELANGE_EXEC_HELPER") != "1" {
		return
	}
	workingDir, err := os.Getwd()
	if err != nil {
		os.Exit(16)
	}
	fmt.Fprintf(os.Stdout, "value=%s\ncwd=%s\n", os.Getenv("MELANGE_EXEC_VALUE"), workingDir)
	fmt.Fprintln(os.Stderr, "sentinel stderr")
	if os.Getenv("MELANGE_EXEC_FAIL") == "1" {
		os.Exit(17)
	}
}

func Test_restrictStagedPrivateKey_handles_missing_directory_and_regular_file(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(t *testing.T, paths Paths)
		wantError error
		contains  string
	}{
		{name: "missing", contains: "staged signing key"},
		{
			name: "directory",
			setup: func(t *testing.T, paths Paths) {
				require.NoError(t, os.MkdirAll(filepath.Join(paths.WorkDir, "melange.rsa"), 0o755))
			},
			wantError: errStagedKeyNotRegular,
		},
		{
			name: "regular file",
			setup: func(t *testing.T, paths Paths) {
				writeTestFile(t, filepath.Join(paths.WorkDir, "melange.rsa"), "private")
				require.NoError(t, os.Chmod(filepath.Join(paths.WorkDir, "melange.rsa"), 0o666))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given
			paths := testPaths(t.TempDir())
			if tt.setup != nil {
				tt.setup(t, paths)
			}

			// When
			err := restrictStagedPrivateKey(&paths)

			// Then
			switch {
			case tt.wantError != nil:
				require.ErrorIs(t, err, tt.wantError)
			case tt.contains != "":
				require.ErrorContains(t, err, tt.contains)
			default:
				require.NoError(t, err)
				info, statErr := os.Stat(filepath.Join(paths.WorkDir, "melange.rsa"))
				require.NoError(t, statErr)
				assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
			}
		})
	}
}

func Test_removeEphemeralSigningKey_removes_files_and_reports_nonempty_directories(t *testing.T) {
	// Given
	paths := testPaths(t.TempDir())
	writeSigningPair(t, &paths)

	// When
	err := removeEphemeralSigningKey(&paths)

	// Then
	require.NoError(t, err)
	assert.NoFileExists(t, filepath.Join(paths.WorkDir, "melange.rsa"))
	assert.NoFileExists(t, filepath.Join(paths.WorkDir, "melange.rsa.pub"))
	require.NoError(t, removeEphemeralSigningKey(&paths))

	for _, name := range []string{"melange.rsa", "melange.rsa.pub"} {
		writeTestFile(t, filepath.Join(paths.WorkDir, name, "child"), "sentinel")
	}
	require.ErrorContains(t, removeEphemeralSigningKey(&paths), "remove ephemeral signing key")
}

func Test_stagedBuildFiles_sorts_recipes_and_reports_glob_boundaries(t *testing.T) {
	// Given
	paths := testPaths(t.TempDir())
	writeTestFile(t, filepath.Join(paths.WorkDir, "specs", "zeta", "build.yaml"), "zeta")
	writeTestFile(t, filepath.Join(paths.WorkDir, "specs", "alpha", "build.yaml"), "alpha")

	// When
	got, err := stagedBuildFiles(&paths)

	// Then
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Contains(t, got[0], "alpha")
	assert.Contains(t, got[1], "zeta")

	bad := paths
	bad.WorkDir = "["
	_, err = stagedBuildFiles(&bad)
	require.ErrorContains(t, err, "find staged recipes")
}

func Test_runBuild_adds_optional_pipeline_environment_and_build_flags(t *testing.T) {
	// Given
	root := t.TempDir()
	paths := testPaths(root)
	require.NoError(t, os.MkdirAll(filepath.Join(paths.WorkDir, "pipelines"), 0o755))
	runner := &orchestrationRunner{}
	options := &BuildOptions{
		Paths:  paths,
		Spec:   Spec{EnvFile: "fips.env", BuildOption: "fips"},
		Arch:   ArchitectureAArch64,
		Runner: runner,
	}
	buildFile := filepath.Join(paths.WorkDir, "specs", "sentinel", "build.yaml")

	// When
	err := runBuild(context.Background(), options, buildFile)

	// Then
	require.NoError(t, err)
	require.Len(t, runner.commands, 1)
	args := strings.Join(runner.commands[0].Args, " ")
	assert.Contains(t, args, "--pipeline-dirs melange-work/pipelines")
	assert.Contains(t, args, "--env-file packages/overrides/fips.env")
	assert.Contains(t, args, "--build-option fips")
	assert.Contains(t, args, "--arch aarch64")
}

func Test_buildAndSign_helpers_stop_on_runner_and_path_errors(t *testing.T) {
	// Given
	root := t.TempDir()
	paths := testPaths(root)
	writeSigningPair(t, &paths)
	builds := []string{
		filepath.Join(paths.WorkDir, "specs", "alpha", "build.yaml"),
		filepath.Join(paths.WorkDir, "specs", "zeta", "build.yaml"),
	}
	buildRunner := &orchestrationRunner{failVerb: "build"}
	buildOptions := &BuildOptions{Paths: paths, Arch: ArchitectureX8664, Runner: buildRunner}

	// When
	buildErr := buildStagedRecipes(context.Background(), buildOptions, builds)

	// Then
	require.ErrorIs(t, buildErr, errSentinelBuildRunner)
	assert.Len(t, buildRunner.commands, 1)

	index := filepath.Join(paths.RepoDir, string(ArchitectureX8664), "APKINDEX.tar.gz")
	writeTestFile(t, index, "index")
	signRunner := &orchestrationRunner{failVerb: "sign-index"}
	signErr := signPackageIndexes(context.Background(), &BuildOptions{
		Paths:  paths,
		Spec:   Spec{Bespoke: []string{"sentinel.yaml"}},
		Arch:   ArchitectureX8664,
		Runner: signRunner,
	})
	require.ErrorIs(t, signErr, errSentinelBuildRunner)

	badPaths := paths
	badPaths.RepoDir = filepath.Join(root, "[")
	globErr := signPackageIndexes(context.Background(), &BuildOptions{
		Paths:  badPaths,
		Arch:   ArchitectureX8664,
		Runner: &orchestrationRunner{},
	})
	require.ErrorContains(t, globErr, "find package indexes")
}
