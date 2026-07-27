package repositoryops

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type gitLockSnapshot struct {
	roots []string
	files map[string]gitFileSnapshot
}

func captureGitLocks(roots []string) (gitLockSnapshot, error) {
	uniqueRoots := uniqueCleanPaths(roots)
	files, err := scanGitLocks(uniqueRoots)
	if err != nil {
		return gitLockSnapshot{}, fmt.Errorf("%w: %w", ErrGitSnapshot, err)
	}
	return gitLockSnapshot{roots: uniqueRoots, files: files}, nil
}

func uniqueCleanPaths(paths []string) []string {
	seen := make(map[string]struct{}, len(paths))
	unique := make([]string, 0, len(paths))
	for _, path := range paths {
		cleaned := filepath.Clean(path)
		if _, exists := seen[cleaned]; exists {
			continue
		}
		seen[cleaned] = struct{}{}
		unique = append(unique, cleaned)
	}
	sort.Strings(unique)
	return unique
}

func scanGitLocks(roots []string) (map[string]gitFileSnapshot, error) {
	locks := make(map[string]gitFileSnapshot)
	for _, root := range roots {
		if !filepath.IsAbs(root) || containsControl(root) {
			return nil, fmt.Errorf("%w: Git directory path %q", ErrUnsupportedGitState, root)
		}
		if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return fmt.Errorf("walk Git metadata %q: %w", path, walkErr)
			}
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".lock") {
				return nil
			}
			snapshot, err := captureGitFile(gitFileSnapshotRequest{path: path, label: "Git lock", required: true})
			if err != nil {
				return err
			}
			locks[snapshot.path] = snapshot
			return nil
		}); err != nil {
			return nil, err
		}
	}
	return locks, nil
}

func (snapshot gitLockSnapshot) removeCreated() (bool, error) {
	current, err := scanGitLocks(snapshot.roots)
	if err != nil {
		return false, err
	}
	removed := false
	var removeErrors []error
	for path := range current {
		if _, existed := snapshot.files[path]; existed {
			continue
		}
		if err := removeGitFile(path); err != nil {
			removeErrors = append(removeErrors, err)
			continue
		}
		removed = true
	}
	return removed, errors.Join(removeErrors...)
}

func (snapshot gitLockSnapshot) restoreExisting() error {
	var restoreErrors []error
	for _, file := range snapshot.files {
		if err := file.restore(); err != nil {
			restoreErrors = append(restoreErrors, err)
		}
	}
	if err := errors.Join(restoreErrors...); err != nil {
		return fmt.Errorf("restore pre-existing Git locks: %w", err)
	}
	return nil
}

func gitDirectoryRoots(paths ...string) ([]string, error) {
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("inspect Git directory %q: %w", path, err)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("%w: Git directory %q is not a directory", ErrUnsupportedGitState, path)
		}
	}
	return uniqueCleanPaths(paths), nil
}
