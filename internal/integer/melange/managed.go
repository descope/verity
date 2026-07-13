package melange

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func validateDirectoryChain(path string) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	volume := filepath.VolumeName(absolute)
	current := volume + string(filepath.Separator)
	remainder := strings.TrimPrefix(absolute, current)
	for part := range strings.SplitSeq(remainder, string(filepath.Separator)) {
		if part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("%w: %s", errInvalidRoot, current)
		}
	}
	return nil
}

func ensureManagedDirectory(rootPath, targetPath string) (*os.Root, string, error) {
	if err := validateDirectoryChain(rootPath); err != nil {
		return nil, "", err
	}
	relative, err := relativeToRoot(rootPath, targetPath)
	if err != nil {
		return nil, "", err
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return nil, "", err
	}
	if err := validateRootDirectories(root, relative, true); err != nil {
		_ = root.Close()
		return nil, "", err
	}
	if err := root.MkdirAll(relative, 0o755); err != nil {
		_ = root.Close()
		return nil, "", err
	}
	if err := validateRootDirectories(root, relative, false); err != nil {
		_ = root.Close()
		return nil, "", err
	}
	return root, relative, nil
}

func relativeToRoot(rootPath, targetPath string) (string, error) {
	rootAbsolute, err := filepath.Abs(rootPath)
	if err != nil {
		return "", err
	}
	targetAbsolute, err := filepath.Abs(targetPath)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(rootAbsolute, targetAbsolute)
	if err != nil {
		return "", err
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w %q", errUnsafeRelativePath, targetPath)
	}
	return relative, nil
}

func validateRootDirectories(root *os.Root, relative string, allowMissing bool) error {
	current := "."
	for part := range strings.SplitSeq(filepath.Clean(relative), string(filepath.Separator)) {
		if part == "." || part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, err := root.Lstat(current)
		if os.IsNotExist(err) && allowMissing {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("%w: %s", errInvalidRoot, current)
		}
	}
	return nil
}

func realDirectory(path string) (bool, error) {
	if err := validateDirectoryChain(path); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
