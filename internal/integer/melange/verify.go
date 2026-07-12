package melange

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

func readVerifiedFile(root, relative, expected string) ([]byte, error) {
	data, err := readRegularFile(root, relative)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(data)
	actual := hex.EncodeToString(sum[:])
	if actual != expected {
		return nil, fmt.Errorf("%w for %s: expected %s, got %s", errChecksumMismatch, relative, expected, actual)
	}
	return data, nil
}

func regularFile(root, relative string) (string, error) {
	path, err := securePath(root, relative)
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%s %w", relative, errNotRegularFile)
	}
	return path, nil
}

func readRegularFile(root, relative string) ([]byte, error) {
	if _, err := regularFile(root, relative); err != nil {
		return nil, err
	}
	openedRoot, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer openedRoot.Close()
	name := filepath.Clean(filepath.FromSlash(relative))
	info, err := openedRoot.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s %w", relative, errNotRegularFile)
	}
	file, err := openedRoot.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err = file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s %w", relative, errNotRegularFile)
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", relative, err)
	}
	return data, nil
}

func secureOptionalDir(root, relative string) (string, error) {
	path, err := securePath(root, relative)
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s %w", relative, errNotRealDirectory)
	}
	return path, nil
}

func securePath(root, relative string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(relative))
	if relative != "" && (filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator))) {
		return "", fmt.Errorf("%w %q", errUnsafeRelativePath, relative)
	}
	if err := validateDirectoryChain(root); err != nil {
		return "", err
	}
	if relative == "" {
		return root, nil
	}
	current := root
	for part := range strings.SplitSeq(clean, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, statErr := os.Lstat(current)
		if os.IsNotExist(statErr) {
			continue
		}
		if statErr != nil {
			return "", statErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("path %s %w", relative, errPathContainsSymlink)
		}
	}
	return filepath.Join(root, clean), nil
}

func treeFiles(root, prefix string) ([]string, error) {
	if root == "" {
		return nil, nil
	}
	exists, err := realDirectory(root)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, nil
	}
	var files []string
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 || (!entry.IsDir() && !info.Mode().IsRegular()) {
			return fmt.Errorf("%s %w", path, errInvalidTreeEntry)
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(filepath.Join(prefix, relative)))
		return nil
	})
	slices.Sort(files)
	return files, err
}

func compareFileSet(label string, expected, actual []string) error {
	slices.Sort(expected)
	if slices.Equal(expected, actual) {
		return nil
	}
	return fmt.Errorf("%s %w: expected %v, got %v", label, errFileSetMismatch, expected, actual)
}

func mapKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}
