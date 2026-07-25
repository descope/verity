package metrics

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type archiveFile struct {
	name    string
	content []byte
}

func copyArchivedMetrics(artifacts []ValidatedArtifact, workspace, targetDir string) error {
	files, err := readValidatedArtifacts(artifacts)
	if err != nil {
		return err
	}
	destination := filepath.Join(workspace, filepath.FromSlash(targetDir))
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return fmt.Errorf("create metrics target: %w", err)
	}
	for index := range files {
		if err := writeArchiveFile(destination, &files[index]); err != nil {
			return err
		}
	}
	return nil
}

func readValidatedArtifacts(artifacts []ValidatedArtifact) ([]archiveFile, error) {
	files := make([]archiveFile, 0, len(artifacts))
	for index := range artifacts {
		artifact := artifacts[index]
		content, err := os.ReadFile(artifact.Path)
		if err != nil {
			return nil, fmt.Errorf("read metrics source %q: %w", artifact.Path, err)
		}
		if sha256.Sum256(content) != artifact.Digest {
			return nil, fmt.Errorf("%w: metrics source %q changed after validation", ErrInvalidMetrics, artifact.Path)
		}
		name := strings.TrimPrefix(strings.TrimSuffix(filepath.Base(artifact.Path), ".json"), "metrics-") + ".json"
		files = append(files, archiveFile{name: name, content: content})
	}
	return files, nil
}

func writeArchiveFile(destination string, file *archiveFile) (retErr error) {
	temporary, err := os.CreateTemp(destination, ".metrics-*")
	if err != nil {
		return fmt.Errorf("create archived metrics %q: %w", file.name, err)
	}
	temporaryPath := temporary.Name()
	closed := false
	defer func() {
		if !closed {
			retErr = errors.Join(retErr, temporary.Close())
		}
		if removeErr := os.Remove(temporaryPath); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			retErr = errors.Join(retErr, fmt.Errorf("remove temporary metrics %q: %w", file.name, removeErr))
		}
	}()
	if err := temporary.Chmod(0o644); err != nil {
		return fmt.Errorf("chmod archived metrics %q: %w", file.name, err)
	}
	if _, err := temporary.Write(file.content); err != nil {
		return fmt.Errorf("write archived metrics %q: %w", file.name, err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close archived metrics %q: %w", file.name, err)
	}
	closed = true
	if err := os.Rename(temporaryPath, filepath.Join(destination, file.name)); err != nil {
		return fmt.Errorf("publish archived metrics %q: %w", file.name, err)
	}
	return nil
}

func writeArchiveNotice(writer io.Writer, message string) error {
	if writer == nil {
		return nil
	}
	if _, err := io.WriteString(writer, message); err != nil {
		return fmt.Errorf("write archive notice: %w", err)
	}
	return nil
}
