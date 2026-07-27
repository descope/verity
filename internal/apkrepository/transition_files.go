package apkrepository

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func RepositoryDigest(repository string) (string, error) {
	info, err := os.Stat(repository)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("%w: %s", errRepositoryNotFound, repository)
	}
	paths, err := managedRepositoryFiles(repository)
	if err != nil {
		return "", err
	}
	digest := sha256.New()
	for _, path := range paths {
		relative, err := filepath.Rel(repository, path)
		if err != nil {
			return "", fmt.Errorf("resolve repository digest path %q: %w", path, err)
		}
		if err := writeDigestField(digest, filepath.ToSlash(relative)); err != nil {
			return "", err
		}
		file, err := os.Open(path)
		if err != nil {
			return "", fmt.Errorf("open repository digest file %q: %w", path, err)
		}
		stat, err := file.Stat()
		if err != nil {
			_ = file.Close()
			return "", fmt.Errorf("stat repository digest file %q: %w", path, err)
		}
		if err := binary.Write(digest, binary.BigEndian, uint64(stat.Size())); err != nil {
			_ = file.Close()
			return "", fmt.Errorf("hash repository file size: %w", err)
		}
		if _, err := io.Copy(digest, file); err != nil {
			_ = file.Close()
			return "", fmt.Errorf("hash repository file %q: %w", path, err)
		}
		if err := file.Close(); err != nil {
			return "", fmt.Errorf("close repository digest file %q: %w", path, err)
		}
	}
	return "sha256:" + hex.EncodeToString(digest.Sum(nil)), nil
}

func managedRepositoryFiles(repository string) ([]string, error) {
	paths := make([]string, 0)
	rootEntries, err := os.ReadDir(repository)
	if err != nil {
		return nil, fmt.Errorf("read repository %q: %w", repository, err)
	}
	for _, entry := range rootEntries {
		if entry.Type().IsRegular() && (entry.Name() == "repository-format" || strings.HasSuffix(entry.Name(), ".rsa.pub")) {
			paths = append(paths, filepath.Join(repository, entry.Name()))
		}
	}
	for _, architecture := range supportedArches {
		directory := filepath.Join(repository, architecture)
		entries, err := os.ReadDir(directory)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read architecture %s: %w", architecture, err)
		}
		for _, entry := range entries {
			if entry.Type().IsRegular() {
				paths = append(paths, filepath.Join(directory, entry.Name()))
			}
		}
	}
	sort.Strings(paths)
	return paths, nil
}

func writeDigestField(digest hash.Hash, value string) error {
	if err := binary.Write(digest, binary.BigEndian, uint32(len(value))); err != nil {
		return fmt.Errorf("hash repository path length: %w", err)
	}
	if _, err := io.WriteString(digest, value); err != nil {
		return fmt.Errorf("hash repository path: %w", err)
	}
	return nil
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open %q: %w", path, err)
	}
	defer file.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return "", fmt.Errorf("hash %q: %w", path, err)
	}
	return "sha256:" + hex.EncodeToString(digest.Sum(nil)), nil
}

type stagedOutput struct {
	path      string
	outputDir string
}

func prepareStagedOutput(outputDir string) (*stagedOutput, error) {
	if err := validateOutputDirectory(outputDir); err != nil {
		return nil, err
	}
	if err := recoverInterruptedOutput(outputDir); err != nil {
		return nil, err
	}
	parent := filepath.Dir(outputDir)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return nil, fmt.Errorf("create output parent: %w", err)
	}
	stage, err := os.MkdirTemp(parent, "."+filepath.Base(outputDir)+".stage-")
	if err != nil {
		return nil, fmt.Errorf("create staged output: %w", err)
	}
	if directoryExists(outputDir) {
		if err := os.Remove(stage); err != nil {
			_ = os.RemoveAll(stage)
			return nil, fmt.Errorf("prepare staged copy: %w", err)
		}
		if err := os.CopyFS(stage, os.DirFS(outputDir)); err != nil {
			_ = os.RemoveAll(stage)
			return nil, fmt.Errorf("copy current output into stage: %w", err)
		}
	}
	if err := os.Chmod(stage, 0o755); err != nil {
		_ = os.RemoveAll(stage)
		return nil, fmt.Errorf("set staged output permissions: %w", err)
	}
	return &stagedOutput{path: stage, outputDir: outputDir}, nil
}

func recoverInterruptedOutput(outputDir string) error {
	backup := outputDir + ".previous"
	outputExists := directoryExists(outputDir)
	backupExists := directoryExists(backup)
	switch {
	case !outputExists && backupExists:
		if err := os.Rename(backup, outputDir); err != nil {
			return fmt.Errorf("restore interrupted publication: %w", err)
		}
	case outputExists && backupExists:
		if err := os.RemoveAll(backup); err != nil {
			return fmt.Errorf("remove completed publication backup: %w", err)
		}
	}
	return nil
}

func (stage *stagedOutput) cleanup() {
	_ = os.RemoveAll(stage.path)
}

func (stage *stagedOutput) commit() error {
	backup := stage.outputDir + ".previous"
	if err := os.RemoveAll(backup); err != nil {
		return fmt.Errorf("remove stale output backup: %w", err)
	}
	hadOutput := directoryExists(stage.outputDir)
	if hadOutput {
		if err := os.Rename(stage.outputDir, backup); err != nil {
			return fmt.Errorf("stage previous output: %w", err)
		}
	}
	if err := os.Rename(stage.path, stage.outputDir); err != nil {
		if hadOutput {
			if rollbackErr := os.Rename(backup, stage.outputDir); rollbackErr != nil {
				return fmt.Errorf("publish staged output: %w (rollback failed: %w)", err, rollbackErr)
			}
		}
		return fmt.Errorf("publish staged output: %w", err)
	}
	stage.path = ""
	if hadOutput {
		if err := os.RemoveAll(backup); err != nil {
			return fmt.Errorf("remove previous output: %w", err)
		}
	}
	return nil
}
