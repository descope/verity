package melange

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errSentinelBuildRunner = errors.New("sentinel build runner failure")

func Test_Build_returns_early_for_empty_spec_and_requires_arch_for_staged_build(t *testing.T) {
	// Given
	root := t.TempDir()

	// When
	emptyErr := Build(context.Background(), &BuildOptions{Paths: testPaths(root)})
	archErr := Build(context.Background(), &BuildOptions{Paths: testPaths(root), Staged: true})

	// Then
	require.NoError(t, emptyErr)
	require.ErrorIs(t, archErr, errArchitectureRequired)
}

func Test_Build_removes_ephemeral_staged_key_after_success(t *testing.T) {
	// Given
	root := t.TempDir()
	paths, spec := setupStagedUpstreamBuild(t, root)
	runner := &fakeRunner{}

	// When
	err := Build(context.Background(), &BuildOptions{
		Paths:  paths,
		Spec:   spec,
		Arch:   ArchitectureX8664,
		Staged: true,
		Runner: runner,
	})

	// Then
	require.NoError(t, err)
	assert.NoFileExists(t, filepath.Join(paths.WorkDir, "melange.rsa"))
	assert.NoFileExists(t, filepath.Join(paths.WorkDir, "melange.rsa.pub"))
	assert.FileExists(t, artifactMarkerPath(&paths, ArchitectureX8664))
}

func Test_Build_removes_ephemeral_staged_key_when_recipe_build_fails(t *testing.T) {
	// Given
	root := t.TempDir()
	paths, spec := setupStagedUpstreamBuild(t, root)
	runner := &orchestrationRunner{failVerb: "build"}

	// When
	err := Build(context.Background(), &BuildOptions{
		Paths:  paths,
		Spec:   spec,
		Arch:   ArchitectureX8664,
		Staged: true,
		Runner: runner,
	})

	// Then
	require.ErrorIs(t, err, errSentinelBuildRunner)
	assert.NoFileExists(t, filepath.Join(paths.WorkDir, "melange.rsa"))
	assert.NoFileExists(t, filepath.Join(paths.WorkDir, "melange.rsa.pub"))
}

func Test_Build_stops_at_missing_recipes_index_and_marker_boundaries(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(t *testing.T, paths Paths) Spec
		runner    Runner
		wantError error
		contains  string
	}{
		{
			name: "no staged recipes",
			setup: func(t *testing.T, paths Paths) Spec {
				writeSigningPair(t, &paths)
				return Spec{Bespoke: []string{"sentinel.yaml"}}
			},
			runner:    &orchestrationRunner{},
			wantError: errNoStagedRecipes,
		},
		{
			name: "build produced no package index",
			setup: func(t *testing.T, paths Paths) Spec {
				writeSigningPair(t, &paths)
				writeTestFile(t, filepath.Join(paths.WorkDir, "specs", "sentinel", "build.yaml"), "package:\n  name: sentinel\n")
				return Spec{Bespoke: []string{"sentinel.yaml"}}
			},
			runner:    &orchestrationRunner{},
			wantError: errNoPackageIndex,
		},
		{
			name: "artifact marker input is missing",
			setup: func(t *testing.T, paths Paths) Spec {
				writeSigningPair(t, &paths)
				writeTestFile(t, filepath.Join(paths.WorkDir, "specs", "sentinel", "build.yaml"), "package:\n  name: sentinel\n")
				return Spec{Bespoke: []string{"missing.yaml"}}
			},
			runner:   &orchestrationRunner{createIndex: true},
			contains: "record built artifacts",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given
			paths := testPaths(t.TempDir())
			spec := tt.setup(t, paths)

			// When
			err := Build(context.Background(), &BuildOptions{
				Paths:  paths,
				Spec:   spec,
				Arch:   ArchitectureX8664,
				Staged: true,
				Runner: tt.runner,
			})

			// Then
			if tt.wantError != nil {
				require.ErrorIs(t, err, tt.wantError)
			} else {
				require.ErrorContains(t, err, tt.contains)
			}
		})
	}
}

type orchestrationRunner struct {
	commands    []Command
	failVerb    string
	createIndex bool
}

func (r *orchestrationRunner) Run(_ context.Context, command *Command, _, _ io.Writer) error {
	r.commands = append(r.commands, *command)
	if len(command.Args) == 0 {
		return nil
	}
	verb := command.Args[0]
	if verb == "keygen" {
		writeRunnerFile(command.Dir, command.Args[1], "private")
		writeRunnerFile(command.Dir, command.Args[1]+".pub", "public")
	}
	if verb == "build" && r.createIndex {
		writeRunnerFile(command.Dir, "packages/repo/x86_64/APKINDEX.tar.gz", "index")
	}
	if verb == r.failVerb {
		return fmt.Errorf("%w: %s", errSentinelBuildRunner, verb)
	}
	return nil
}

func setupStagedUpstreamBuild(t *testing.T, root string) (Paths, Spec) {
	t.Helper()
	paths := testPaths(root)
	recipe := "package:\n  name: sentinel\n"
	writeTestFile(t, filepath.Join(paths.LockedDir, "sentinel.yaml"), recipe)
	writeTestFile(t, paths.LockFile, fmt.Sprintf(`{"packages":{"sentinel":{"file":"sentinel.yaml","sha256":%q,"assets":{}}},"pipeline_files":{}}`, testSHA(recipe)))
	writeTestFile(t, filepath.Join(paths.WorkDir, "specs", "sentinel", "build.yaml"), recipe)
	return paths, Spec{Upstream: "sentinel"}
}

func writeSigningPair(t *testing.T, paths *Paths) {
	t.Helper()
	writeTestFile(t, filepath.Join(paths.WorkDir, "melange.rsa"), "private")
	writeTestFile(t, filepath.Join(paths.WorkDir, "melange.rsa.pub"), "public")
}
