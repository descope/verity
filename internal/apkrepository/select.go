package apkrepository

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type SelectOptions struct {
	CandidateDir string
	PreviousDir  string
	OutputDir    string
	Stdout       io.Writer
}

func Select(ctx context.Context, options *SelectOptions) error {
	if options == nil {
		return errOptionsRequired
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("select APK repository: %w", err)
	}
	if info, err := os.Stat(options.CandidateDir); err != nil || !info.IsDir() {
		return fmt.Errorf("%w: %s", errCandidateNotFound, options.CandidateDir)
	}
	if err := validateOutputDirectory(options.OutputDir); err != nil {
		return err
	}
	candidateState, err := repositoryState(options.CandidateDir)
	if err != nil {
		return err
	}
	previousState, err := repositoryState(options.PreviousDir)
	if err != nil {
		return err
	}
	selectedDir := options.CandidateDir
	stdout := writerOrDiscard(options.Stdout)
	if directoryExists(options.PreviousDir) && maps.Equal(candidateState, previousState) {
		selectedDir = options.PreviousDir
		fmt.Fprintln(stdout, "APK repository state unchanged; preserving previously published package and index bytes")
	} else {
		fmt.Fprintln(stdout, "APK repository state changed; publishing the newly assembled repository")
	}
	return copySelectedRepository(selectedDir, options.OutputDir)
}

func repositoryState(repository string) (map[string][sha256.Size]byte, error) {
	state := make(map[string][sha256.Size]byte)
	if !directoryExists(repository) {
		return state, nil
	}
	for _, architecture := range supportedArches {
		packages, err := filepath.Glob(filepath.Join(repository, architecture, "*.apk"))
		if err != nil {
			return nil, fmt.Errorf("list %s packages: %w", architecture, err)
		}
		sort.Strings(packages)
		for _, packagePath := range packages {
			if err := addFileDigest(state, repository, packagePath); err != nil {
				return nil, err
			}
		}
	}
	rootFiles, err := os.ReadDir(repository)
	if err != nil {
		return nil, fmt.Errorf("read repository %q: %w", repository, err)
	}
	for _, entry := range rootFiles {
		if entry.Type().IsRegular() && (strings.HasSuffix(entry.Name(), ".rsa.pub") || entry.Name() == "repository-format") {
			if err := addFileDigest(state, repository, filepath.Join(repository, entry.Name())); err != nil {
				return nil, err
			}
		}
	}
	return state, nil
}

func addFileDigest(state map[string][sha256.Size]byte, root, path string) error {
	contents, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read repository state %q: %w", path, err)
	}
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return fmt.Errorf("resolve repository state path %q: %w", path, err)
	}
	state[filepath.ToSlash(relative)] = sha256.Sum256(contents)
	return nil
}

func copySelectedRepository(selectedDir, outputDir string) error {
	if err := cleanSelectedOutput(outputDir); err != nil {
		return err
	}
	if err := copySelectedArchitectures(selectedDir, outputDir); err != nil {
		return err
	}
	return copySelectedMetadata(selectedDir, outputDir)
}

func cleanSelectedOutput(outputDir string) error {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	for _, name := range []string{".no-apks-found", "repository-format"} {
		if err := removeIfExists(filepath.Join(outputDir, name)); err != nil {
			return err
		}
	}
	rootEntries, err := os.ReadDir(outputDir)
	if err != nil {
		return fmt.Errorf("read output directory: %w", err)
	}
	for _, entry := range rootEntries {
		if entry.Type().IsRegular() && strings.HasSuffix(entry.Name(), ".rsa.pub") {
			if err := os.Remove(filepath.Join(outputDir, entry.Name())); err != nil {
				return fmt.Errorf("remove old public key %q: %w", entry.Name(), err)
			}
		}
	}
	return nil
}

func copySelectedArchitectures(selectedDir, outputDir string) error {
	for _, architecture := range supportedArches {
		destination := filepath.Join(outputDir, architecture)
		if err := os.RemoveAll(destination); err != nil {
			return fmt.Errorf("remove output architecture %s: %w", architecture, err)
		}
		source := filepath.Join(selectedDir, architecture)
		if directoryExists(source) {
			if err := os.CopyFS(destination, os.DirFS(source)); err != nil {
				return fmt.Errorf("copy selected architecture %s: %w", architecture, err)
			}
		}
	}
	return nil
}

func copySelectedMetadata(selectedDir, outputDir string) error {
	publicKeys, err := filepath.Glob(filepath.Join(selectedDir, "*.rsa.pub"))
	if err != nil {
		return fmt.Errorf("list selected public keys: %w", err)
	}
	if len(publicKeys) == 0 {
		return errNoPublicKey
	}
	sort.Strings(publicKeys)
	for _, publicKey := range publicKeys {
		if err := copyFile(publicKey, filepath.Join(outputDir, filepath.Base(publicKey))); err != nil {
			return err
		}
	}
	formatPath := filepath.Join(selectedDir, "repository-format")
	if _, err := os.Stat(formatPath); err == nil {
		return copyFile(formatPath, filepath.Join(outputDir, "repository-format"))
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat repository format: %w", err)
	}
	return nil
}

func directoryExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
