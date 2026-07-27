package sitepublication

import (
	"fmt"
	"os"
	"path/filepath"
)

func prepareSiteStage(outputDir string) (string, error) {
	parent := filepath.Dir(outputDir)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return "", fmt.Errorf("create site output parent: %w", err)
	}
	stage, err := os.MkdirTemp(parent, ".site-publication-stage-")
	if err != nil {
		return "", fmt.Errorf("create site stage: %w", err)
	}
	return stage, nil
}

func writeSiteBytes(root, relative string, data []byte, mode os.FileMode) error {
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create metadata directory: %w", err)
	}
	if err := os.WriteFile(path, data, mode); err != nil {
		return fmt.Errorf("write site metadata %q: %w", relative, err)
	}
	return nil
}

func replaceSiteDirectory(stage, outputDir string) error {
	backup := outputDir + ".previous"
	if err := os.RemoveAll(backup); err != nil {
		return fmt.Errorf("remove stale site backup: %w", err)
	}
	hadOutput := directoryExists(outputDir)
	if hadOutput {
		if err := os.Rename(outputDir, backup); err != nil {
			return fmt.Errorf("stage previous site: %w", err)
		}
	}
	if err := os.Rename(stage, outputDir); err != nil {
		if hadOutput {
			if rollbackErr := os.Rename(backup, outputDir); rollbackErr != nil {
				return fmt.Errorf("publish assembled site: %w (rollback failed: %w)", err, rollbackErr)
			}
		}
		return fmt.Errorf("publish assembled site: %w", err)
	}
	if hadOutput {
		if err := os.RemoveAll(backup); err != nil {
			return fmt.Errorf("remove previous site: %w", err)
		}
	}
	return nil
}

func directoryExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
