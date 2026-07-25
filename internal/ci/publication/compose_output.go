package publication

import (
	"fmt"
	"os"
	"path/filepath"
)

func WriteComposeOutputs(publicationPath, componentsPath string, result *ComposeResult) error {
	if result == nil || publicationPath == "" || componentsPath == "" || publicationPath == componentsPath {
		return fmt.Errorf("%w: distinct explicit output paths are required", ErrComposeInvalid)
	}
	componentsTemp, err := stageOutput(componentsPath, result.ComponentsJSON)
	if err != nil {
		return err
	}
	defer os.Remove(componentsTemp)
	publicationTemp, err := stageOutput(publicationPath, result.PublicationJSON)
	if err != nil {
		return err
	}
	defer os.Remove(publicationTemp)
	if err := os.Rename(componentsTemp, componentsPath); err != nil {
		return fmt.Errorf("publish components output %q: %w", componentsPath, err)
	}
	if err := os.Rename(publicationTemp, publicationPath); err != nil {
		return fmt.Errorf("publish publication output %q: %w", publicationPath, err)
	}
	return nil
}

func stageOutput(path string, data []byte) (temporaryPath string, err error) {
	directory := filepath.Dir(path)
	file, err := os.CreateTemp(directory, ".publication-compose-*")
	if err != nil {
		return "", fmt.Errorf("stage output %q: %w", path, err)
	}
	temporaryPath = file.Name()
	defer func() {
		closeErr := file.Close()
		if err == nil && closeErr != nil {
			err = fmt.Errorf("close staged output %q: %w", path, closeErr)
		}
		if err != nil {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := file.Chmod(0o600); err != nil {
		return "", fmt.Errorf("chmod staged output %q: %w", path, err)
	}
	if _, err := file.Write(data); err != nil {
		return "", fmt.Errorf("write staged output %q: %w", path, err)
	}
	if err := file.Sync(); err != nil {
		return "", fmt.Errorf("sync staged output %q: %w", path, err)
	}
	return temporaryPath, nil
}
