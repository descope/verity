package melange

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
)

type ExecRunner struct{}

var errIncompleteSigningKeyPair = errors.New("incomplete ephemeral signing key pair")

func signingKeyName(options *BuildOptions) string {
	return "melange-" + string(options.Arch) + ".rsa"
}

func signingKeyPath(options *BuildOptions) string {
	return filepath.ToSlash(filepath.Join("melange-work", signingKeyName(options)))
}

func (ExecRunner) Run(ctx context.Context, command *Command, stdout, stderr io.Writer) error {
	cmd := exec.CommandContext(ctx, command.Name, command.Args...)
	cmd.Dir = command.Dir
	cmd.Env = append(os.Environ(), command.Env...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

func Prepare(ctx context.Context, options *BuildOptions) error {
	buildDefaults(options)
	if !options.Spec.Needed() {
		return nil
	}
	if err := Stage(&options.Paths, options.Spec); err != nil {
		return err
	}
	return nil
}

func signingKeyPairExists(paths *Paths) (bool, error) {
	return signingKeyPairExistsNamed(paths, "melange.rsa")
}

func signingKeyPairExistsNamed(paths *Paths, privateName string) (bool, error) {
	found := 0
	for _, name := range []string{privateName, privateName + ".pub"} {
		path := filepath.Join(paths.WorkDir, name)
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return false, fmt.Errorf("inspect signing key %q: %w", path, err)
		}
		if !info.Mode().IsRegular() {
			return false, fmt.Errorf("signing key %q %w", path, errNotRegularFile)
		}
		found++
	}
	if found == 1 {
		return false, errIncompleteSigningKeyPair
	}
	return found == 2, nil
}

func Build(ctx context.Context, options *BuildOptions) (err error) {
	buildDefaults(options)
	if !options.Spec.Needed() && !options.Staged {
		return nil
	}
	if options.Arch == "" {
		return errArchitectureRequired
	}
	if !options.Arch.valid() {
		return fmt.Errorf("%w %q", errUnsupportedArchitecture, options.Arch)
	}
	generatedKey, err := prepareBuildInputs(ctx, options)
	if err != nil {
		return err
	}
	if generatedKey {
		defer func() {
			err = errors.Join(err, removeEphemeralSigningKeyNamed(&options.Paths, signingKeyName(options)))
		}()
	}
	if err := clearBuildOutput(&options.Paths, options.Arch); err != nil {
		return err
	}
	builds, err := stagedBuildFiles(&options.Paths)
	if err != nil {
		return err
	}
	if err := buildStagedRecipes(ctx, options, builds); err != nil {
		return err
	}
	return signPackageIndexes(ctx, options)
}

func clearBuildOutput(paths *Paths, arch Architecture) error {
	root, repository, err := ensureManagedDirectory(paths.Root, paths.RepoDir)
	if err != nil {
		return fmt.Errorf("prepare package repository: %w", err)
	}
	defer root.Close()
	if err := root.RemoveAll(filepath.Join(repository, string(arch))); err != nil {
		return fmt.Errorf("clear package build output: %w", err)
	}
	return nil
}

func prepareBuildInputs(ctx context.Context, options *BuildOptions) (bool, error) {
	if !options.Staged {
		if err := Stage(&options.Paths, options.Spec); err != nil {
			return false, err
		}
	}
	privateName := signingKeyName(options)
	keyPairExists, err := signingKeyPairExistsNamed(&options.Paths, privateName)
	if err != nil {
		return false, err
	}
	if keyPairExists {
		return false, restrictStagedPrivateKeyNamed(&options.Paths, privateName)
	}
	if err := runMelange(ctx, options, "keygen", signingKeyPath(options)); err != nil {
		return false, fmt.Errorf("generate ephemeral package signing key: %w", err)
	}
	return true, restrictStagedPrivateKeyNamed(&options.Paths, privateName)
}

func restrictStagedPrivateKey(paths *Paths) error {
	return restrictStagedPrivateKeyNamed(paths, "melange.rsa")
}

func restrictStagedPrivateKeyNamed(paths *Paths, privateName string) error {
	privateKey := filepath.Join(paths.WorkDir, privateName)
	info, err := os.Lstat(privateKey)
	if err != nil {
		return fmt.Errorf("staged signing key: %w", err)
	}
	if !info.Mode().IsRegular() {
		return errStagedKeyNotRegular
	}
	if err := os.Chmod(privateKey, 0o600); err != nil {
		return fmt.Errorf("restrict staged signing key: %w", err)
	}
	return nil
}

func removeEphemeralSigningKey(paths *Paths) error {
	return removeEphemeralSigningKeyNamed(paths, "melange.rsa")
}

func removeEphemeralSigningKeyNamed(paths *Paths, privateName string) error {
	var result error
	for _, name := range []string{privateName, privateName + ".pub"} {
		path := filepath.Join(paths.WorkDir, name)
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			result = errors.Join(result, fmt.Errorf("remove ephemeral signing key %q: %w", path, err))
		}
	}
	return result
}

func stagedBuildFiles(paths *Paths) ([]string, error) {
	builds, err := filepath.Glob(filepath.Join(paths.WorkDir, "specs", "*", "build.yaml"))
	if err != nil {
		return nil, fmt.Errorf("find staged recipes: %w", err)
	}
	slices.Sort(builds)
	if len(builds) == 0 {
		return nil, errNoStagedRecipes
	}
	return builds, nil
}

func buildStagedRecipes(ctx context.Context, options *BuildOptions, builds []string) error {
	for _, buildFile := range builds {
		if err := runBuild(ctx, options, buildFile); err != nil {
			return err
		}
	}
	return nil
}

func signPackageIndexes(ctx context.Context, options *BuildOptions) error {
	privateKey := signingKeyPath(options)
	publicKey := filepath.Join(options.Paths.Root, filepath.FromSlash(privateKey+".pub"))
	publicName := filepath.Base(privateKey) + ".pub"
	if publicName != "melange.rsa.pub" {
		if err := copyFile(options.Paths.Root, publicKey, filepath.Join(options.Paths.RepoDir, publicName)); err != nil {
			return fmt.Errorf("copy public key: %w", err)
		}
	}
	if err := copyFile(options.Paths.Root, publicKey, filepath.Join(options.Paths.RepoDir, "melange.rsa.pub")); err != nil {
		return fmt.Errorf("copy compatibility public key: %w", err)
	}
	indexes, err := filepath.Glob(filepath.Join(options.Paths.RepoDir, string(options.Arch), "APKINDEX.tar.gz"))
	if err != nil {
		return fmt.Errorf("find package indexes: %w", err)
	}
	slices.Sort(indexes)
	if len(indexes) == 0 {
		return errNoPackageIndex
	}
	for _, index := range indexes {
		relative, err := filepath.Rel(options.Paths.Root, index)
		if err != nil {
			return err
		}
		if err := runMelange(ctx, options, "sign-index", "--signing-key", privateKey, filepath.ToSlash(relative), "--force"); err != nil {
			return fmt.Errorf("sign package index: %w", err)
		}
	}
	if err := writeArtifactMarker(&options.Paths, options.Spec, options.Arch); err != nil {
		return fmt.Errorf("record built artifacts: %w", err)
	}
	return nil
}

func runBuild(ctx context.Context, options *BuildOptions, buildFile string) error {
	relative, err := filepath.Rel(options.Paths.Root, buildFile)
	if err != nil {
		return err
	}
	args := []string{
		"build", filepath.ToSlash(relative),
		"--arch", string(options.Arch),
		"--signing-key", signingKeyPath(options),
		"--out-dir", "packages/repo",
		"--repository-append", RepositoryURL,
		"--keyring-append", KeyringURL,
		"--runner", "docker",
	}
	if info, err := os.Stat(filepath.Join(options.Paths.WorkDir, "pipelines")); err == nil && info.IsDir() {
		args = append(args, "--pipeline-dirs", "melange-work/pipelines")
	}
	if options.Spec.EnvFile != "" {
		args = append(args, "--env-file", filepath.ToSlash(filepath.Join("packages", "overrides", options.Spec.EnvFile)))
	}
	if options.Spec.BuildOption != "" {
		args = append(args, "--build-option", options.Spec.BuildOption)
	}
	if err := runMelange(ctx, options, args...); err != nil {
		return fmt.Errorf("build %s: %w", filepath.Base(filepath.Dir(buildFile)), err)
	}
	return nil
}

func runMelange(ctx context.Context, options *BuildOptions, args ...string) error {
	return options.Runner.Run(ctx, &Command{Name: "melange", Args: args, Dir: options.Paths.Root}, options.Stdout, options.Stderr)
}

func buildDefaults(options *BuildOptions) {
	if options.Runner == nil {
		options.Runner = ExecRunner{}
	}
	if options.Stdout == nil {
		options.Stdout = os.Stdout
	}
	if options.Stderr == nil {
		options.Stderr = os.Stderr
	}
}
