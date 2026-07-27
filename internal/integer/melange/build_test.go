package melange

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordedCommand struct {
	name string
	args []string
}

type fakeRunner struct {
	commands  []recordedCommand
	beforeRun func(Command)
	keyMode   os.FileMode
}

func (r *fakeRunner) Run(_ context.Context, command *Command, _, _ io.Writer) error {
	if r.beforeRun != nil {
		r.beforeRun(*command)
	}
	r.commands = append(r.commands, recordedCommand{name: command.Name, args: append([]string(nil), command.Args...)})
	switch {
	case command.Name == "melange" && len(command.Args) > 0 && command.Args[0] == "keygen":
		writeRunnerFile(command.Dir, command.Args[1], "private")
		writeRunnerFile(command.Dir, command.Args[1]+".pub", "public")
		if r.keyMode != 0 {
			if err := os.Chmod(filepath.Join(command.Dir, command.Args[1]), r.keyMode); err != nil {
				panic(err)
			}
		}
	case command.Name == "melange" && len(command.Args) > 0 && command.Args[0] == "build":
		writeRunnerFile(command.Dir, "packages/repo/x86_64/APKINDEX.tar.gz", "index")
	}
	return nil
}

func writeRunnerFile(root, relative, body string) {
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		panic(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		panic(err)
	}
}

func TestBuildStagesRunsAndSigns(t *testing.T) {
	root := t.TempDir()
	recipe := "package:\n  name: caddy\n"
	writeTestFile(t, testPath(root, "packages/bespoke/locked/caddy.yaml"), recipe)
	writeTestFile(t, testPath(root, "packages/upstream.lock.json"), fmt.Sprintf(`{
  "packages":{"caddy":{"file":"caddy.yaml","sha256":"%s","assets":{}}},
  "pipeline_files":{}
}`, testSHA(recipe)))
	writeTestFile(t, testPath(root, "packages/overrides/fips.env"), "GOFIPS140=v1.0.0\n")
	stalePackage := testPath(root, "packages/repo/x86_64/stale.apk")
	writeTestFile(t, stalePackage, "stale")
	runner := &fakeRunner{}
	paths := testPaths(root)
	spec := Spec{Upstream: "caddy", EnvFile: "fips.env", BuildOption: "fips"}

	err := Build(context.Background(), &BuildOptions{
		Paths:  paths,
		Spec:   spec,
		Arch:   ArchitectureX8664,
		Runner: runner,
	})
	require.NoError(t, err)

	require.Len(t, runner.commands, 3)
	assert.Equal(t, recordedCommand{name: "melange", args: []string{"keygen", "melange-work/melange-x86_64.rsa"}}, runner.commands[0])
	assert.Contains(t, runner.commands[1].args, "--env-file")
	assert.Contains(t, runner.commands[1].args, "packages/overrides/fips.env")
	assert.Contains(t, runner.commands[1].args, "--build-option")
	assert.Contains(t, runner.commands[1].args, "fips")
	assert.Equal(t, "sign-index", runner.commands[2].args[0])

	publicKey, err := os.ReadFile(testPath(root, "packages/repo/melange.rsa.pub"))
	require.NoError(t, err)
	assert.Equal(t, "public", string(publicKey))
	assert.True(t, ArtifactsExist(&paths, spec, ArchitectureX8664))
	assert.NoFileExists(t, stalePackage)
}

func TestBuildSignsOnlyRequestedArchitectureIndex(t *testing.T) {
	root := t.TempDir()
	paths := testPaths(root)
	recipe := "package:\n  name: caddy\n"
	writeTestFile(t, testPath(root, "packages/bespoke/locked/caddy.yaml"), recipe)
	writeTestFile(t, testPath(root, "packages/upstream.lock.json"), fmt.Sprintf(`{
  "packages":{"caddy":{"file":"caddy.yaml","sha256":"%s","assets":{}}},
  "pipeline_files":{}
}`, testSHA(recipe)))
	writeTestFile(t, testPath(paths.WorkDir, "specs/caddy/build.yaml"), recipe)
	writeTestFile(t, filepath.Join(paths.WorkDir, "melange-x86_64.rsa"), "private")
	writeTestFile(t, filepath.Join(paths.WorkDir, "melange-x86_64.rsa.pub"), "public")
	writeTestFile(t, testPath(root, "packages/repo/aarch64/APKINDEX.tar.gz"), "arm-index")
	runner := &fakeRunner{}

	err := Build(context.Background(), &BuildOptions{
		Paths:  paths,
		Spec:   Spec{Upstream: "caddy"},
		Arch:   ArchitectureX8664,
		Staged: true,
		Runner: runner,
	})
	require.NoError(t, err)

	var signedIndexes []string
	for _, command := range runner.commands {
		if len(command.args) > 4 && command.args[0] == "sign-index" {
			signedIndexes = append(signedIndexes, command.args[3])
		}
	}
	assert.Equal(t, []string{"packages/repo/x86_64/APKINDEX.tar.gz"}, signedIndexes)
}

func TestPrepareStages_withoutGeneratingSigningKey(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, testPath(root, "packages/bespoke/custom.yaml"), "package:\n  name: custom\n")
	writeTestFile(t, testPath(root, "packages/upstream.lock.json"), `{"packages":{},"pipeline_files":{}}`)
	runner := &fakeRunner{}

	err := Prepare(context.Background(), &BuildOptions{
		Paths:  testPaths(root),
		Spec:   Spec{Bespoke: []string{"custom.yaml"}},
		Runner: runner,
	})

	require.NoError(t, err)
	assert.Empty(t, runner.commands)
	assert.NoFileExists(t, testPath(root, "melange-work/melange.rsa"))
}

func TestPrepareStagesIdempotently_withoutSigningKeys(t *testing.T) {
	// Given: one workspace whose recipes will be built independently per architecture.
	root := t.TempDir()
	writeTestFile(t, testPath(root, "packages/bespoke/custom.yaml"), "package:\n  name: custom\n")
	writeTestFile(t, testPath(root, "packages/upstream.lock.json"), `{"packages":{},"pipeline_files":{}}`)
	runner := &fakeRunner{}
	options := &BuildOptions{
		Paths:  testPaths(root),
		Spec:   Spec{Bespoke: []string{"custom.yaml"}},
		Runner: runner,
	}

	// When: preparation runs repeatedly before architecture builds.
	require.NoError(t, Prepare(context.Background(), options))
	require.NoError(t, Prepare(context.Background(), options))
	assert.Empty(t, runner.commands)
	assert.NoFileExists(t, testPath(root, "melange-work/melange.rsa"))
}

func TestBuildRejectsUnsafeArchitectureBeforeRemovingOutput(t *testing.T) {
	root := t.TempDir()
	sentinel := testPath(root, "victim/sentinel")
	writeTestFile(t, sentinel, "keep")

	err := Build(context.Background(), &BuildOptions{
		Paths:  testPaths(root),
		Spec:   Spec{Bespoke: []string{"custom.yaml"}},
		Arch:   Architecture("../../../victim"),
		Staged: true,
	})

	require.ErrorIs(t, err, errUnsupportedArchitecture)
	assert.FileExists(t, sentinel)
}

func TestBuildRejectsSymlinkedRepositoryAncestorBeforeRemovingOutput(t *testing.T) {
	root := t.TempDir()
	paths := testPaths(root)
	backing := testPath(root, "external/packages")
	sentinel := filepath.Join(backing, "repo", string(ArchitectureX8664), "sentinel")
	writeTestFile(t, sentinel, "keep")
	require.NoError(t, os.Symlink(backing, testPath(root, "packages")))
	writeTestFile(t, testPath(paths.WorkDir, "specs/custom/build.yaml"), "package:\n  name: custom\n")
	writeTestFile(t, filepath.Join(paths.WorkDir, "melange-x86_64.rsa"), "private")
	writeTestFile(t, filepath.Join(paths.WorkDir, "melange-x86_64.rsa.pub"), "public")

	err := Build(context.Background(), &BuildOptions{
		Paths:  paths,
		Spec:   Spec{Bespoke: []string{"custom.yaml"}},
		Arch:   ArchitectureX8664,
		Staged: true,
	})

	require.Error(t, err)
	assert.FileExists(t, sentinel)
}

func TestBuildStagedRestrictsPrivateKeyPermissions(t *testing.T) {
	// Given: staged artifacts restored with permissive file modes.
	root := t.TempDir()
	paths := testPaths(root)
	recipe := "package:\n  name: caddy\n"
	writeTestFile(t, testPath(root, "packages/bespoke/locked/caddy.yaml"), recipe)
	writeTestFile(t, testPath(root, "packages/upstream.lock.json"), fmt.Sprintf(`{
  "packages":{"caddy":{"file":"caddy.yaml","sha256":"%s","assets":{}}},
  "pipeline_files":{}
}`, testSHA(recipe)))
	writeTestFile(t, testPath(paths.WorkDir, "specs/caddy/build.yaml"), "package:\n  name: caddy\n")
	writeTestFile(t, filepath.Join(paths.WorkDir, "melange-x86_64.rsa"), "private")
	writeTestFile(t, filepath.Join(paths.WorkDir, "melange-x86_64.rsa.pub"), "public")
	require.NoError(t, os.Chmod(filepath.Join(paths.WorkDir, "melange-x86_64.rsa"), 0o644))
	var mode os.FileMode
	runner := &fakeRunner{beforeRun: func(command Command) {
		if len(command.Args) == 0 || command.Args[0] != "build" {
			return
		}
		info, err := os.Stat(filepath.Join(paths.WorkDir, "melange-x86_64.rsa"))
		require.NoError(t, err)
		mode = info.Mode().Perm()
	}}

	// When: the native architecture build consumes the staged artifacts.
	err := Build(context.Background(), &BuildOptions{
		Paths:  paths,
		Spec:   Spec{Upstream: "caddy"},
		Arch:   ArchitectureX8664,
		Staged: true,
		Runner: runner,
	})

	// Then: the private key is restricted before the external build command runs.
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), mode)
}
