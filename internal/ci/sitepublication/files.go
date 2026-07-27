package sitepublication

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/verity-org/verity/internal/ci/publication"
)

type treeFile struct {
	relative string
	path     string
	mode     os.FileMode
}

func listTreeFiles(root string) ([]treeFile, error) {
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("directory %q: %w", root, err)
	}
	files := make([]treeFile, 0)
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: symlink %s", ErrInvalidAssembly, path)
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("%w: unsupported file %s", ErrInvalidAssembly, path)
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return fmt.Errorf("resolve site path %q: %w", path, err)
		}
		relative, err = safeRelative(filepath.ToSlash(relative))
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("stat site file %q: %w", path, err)
		}
		files = append(files, treeFile{relative: relative, path: path, mode: normalizedMode(info.Mode())})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].relative < files[j].relative })
	return files, nil
}

func copyTree(source, destination string) error {
	files, err := listTreeFiles(source)
	if err != nil {
		return err
	}
	for _, file := range files {
		if err := copySiteFile(file.path, filepath.Join(destination, filepath.FromSlash(file.relative)), file.mode); err != nil {
			return err
		}
	}
	return nil
}

func copySiteFile(source, destination string, mode os.FileMode) (resultErr error) {
	input, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open site source %q: %w", source, err)
	}
	defer func() { resultErr = errors.Join(resultErr, input.Close()) }()
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return fmt.Errorf("create site directory: %w", err)
	}
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return fmt.Errorf("create site file %q: %w", destination, err)
	}
	defer func() { resultErr = errors.Join(resultErr, output.Close()) }()
	if _, err := io.Copy(output, input); err != nil {
		return fmt.Errorf("copy site file %q: %w", destination, err)
	}
	return nil
}

func fileDigest(path string) (publication.Digest, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open %q: %w", path, err)
	}
	defer file.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return "", fmt.Errorf("hash %q: %w", path, err)
	}
	return publication.Digest("sha256:" + hex.EncodeToString(digest.Sum(nil))), nil
}

func safeRelative(value string) (string, error) {
	if value == "" || strings.Contains(value, "\\") || filepath.IsAbs(value) {
		return "", fmt.Errorf("%w: unsafe path %q", ErrInvalidAssembly, value)
	}
	clean := filepath.ToSlash(filepath.Clean(value))
	if clean == "." || clean != value || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("%w: unsafe path %q", ErrInvalidAssembly, value)
	}
	return clean, nil
}

func normalizedMode(mode os.FileMode) os.FileMode {
	if mode.Perm()&0o111 != 0 {
		return 0o755
	}
	return 0o644
}
