package repositoryops

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strings"
)

var (
	ErrWorktreeSnapshot         = errors.New("capture worktree snapshot")
	ErrWorktreeRollback         = errors.New("restore worktree snapshot")
	ErrUnsupportedWorktreeEntry = errors.New("unsupported worktree entry")
)

type worktreeEntryKind uint8

const (
	worktreeDirectory worktreeEntryKind = iota + 1
	worktreeRegularFile
	worktreeSymlink
)

type worktreeEntry struct {
	kind       worktreeEntryKind
	mode       fs.FileMode
	data       []byte
	linkTarget string
}

type worktreeSnapshot struct {
	root    string
	entries map[string]worktreeEntry
}

func captureWorktree(rootPath string) (snapshot worktreeSnapshot, err error) {
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return worktreeSnapshot{}, fmt.Errorf("%w: open root: %w", ErrWorktreeSnapshot, err)
	}
	defer func() {
		if closeErr := root.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("%w: close root: %w", ErrWorktreeSnapshot, closeErr))
		}
	}()
	snapshot = worktreeSnapshot{root: rootPath, entries: make(map[string]worktreeEntry)}
	err = fs.WalkDir(root.FS(), ".", func(relative string, directoryEntry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("walk %q: %w", relative, walkErr)
		}
		if relative == "." {
			return nil
		}
		if relative == ".git" {
			if directoryEntry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		info, err := root.Lstat(relative)
		if err != nil {
			return fmt.Errorf("inspect %q: %w", relative, err)
		}
		entry := worktreeEntry{mode: info.Mode()}
		switch {
		case info.IsDir():
			entry.kind = worktreeDirectory
		case info.Mode().IsRegular():
			entry.kind = worktreeRegularFile
			entry.data, err = root.ReadFile(relative)
			if err != nil {
				return fmt.Errorf("read %q: %w", relative, err)
			}
		case info.Mode()&fs.ModeSymlink != 0:
			entry.kind = worktreeSymlink
			entry.linkTarget, err = root.Readlink(relative)
			if err != nil {
				return fmt.Errorf("read symlink %q: %w", relative, err)
			}
		default:
			return fmt.Errorf("%w: %q has mode %s", ErrUnsupportedWorktreeEntry, relative, info.Mode())
		}
		snapshot.entries[relative] = entry
		return nil
	})
	if err != nil {
		return worktreeSnapshot{}, fmt.Errorf("%w: %w", ErrWorktreeSnapshot, err)
	}
	return snapshot, nil
}

func (snapshot worktreeSnapshot) restore() (err error) {
	root, err := os.OpenRoot(snapshot.root)
	if err != nil {
		return fmt.Errorf("open rollback root: %w", err)
	}
	defer func() {
		if closeErr := root.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close rollback root: %w", closeErr))
		}
	}()
	current, err := snapshot.currentPaths(root)
	if err != nil {
		return err
	}
	sort.Slice(current, func(left, right int) bool {
		return pathDepth(current[left]) > pathDepth(current[right])
	})
	for _, relative := range current {
		if _, present := snapshot.entries[relative]; present {
			continue
		}
		if err := root.RemoveAll(relative); err != nil {
			return fmt.Errorf("remove created path %q: %w", relative, err)
		}
	}
	directories := snapshot.pathsOfKind(worktreeDirectory)
	sort.Slice(directories, func(left, right int) bool {
		return pathDepth(directories[left]) < pathDepth(directories[right])
	})
	for _, relative := range directories {
		if err := snapshot.prepareDirectory(root, relative); err != nil {
			return err
		}
	}
	for relative, entry := range snapshot.entries {
		if entry.kind == worktreeDirectory {
			continue
		}
		if err := snapshot.restoreFile(root, relative, entry); err != nil {
			return err
		}
	}
	sort.Slice(directories, func(left, right int) bool {
		return pathDepth(directories[left]) > pathDepth(directories[right])
	})
	for _, relative := range directories {
		if err := chmodRootEntry(root, relative, snapshot.entries[relative].mode.Perm()); err != nil {
			return fmt.Errorf("restore directory mode %q: %w", relative, err)
		}
	}
	return nil
}

func (snapshot worktreeSnapshot) currentPaths(root *os.Root) ([]string, error) {
	paths := make([]string, 0, len(snapshot.entries))
	err := fs.WalkDir(root.FS(), ".", func(relative string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("walk current worktree %q: %w", relative, walkErr)
		}
		if relative == "." {
			return nil
		}
		if relative == ".git" {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		paths = append(paths, relative)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list current worktree: %w", err)
	}
	return paths, nil
}

func (snapshot worktreeSnapshot) pathsOfKind(kind worktreeEntryKind) []string {
	paths := make([]string, 0)
	for relative, entry := range snapshot.entries {
		if entry.kind == kind {
			paths = append(paths, relative)
		}
	}
	return paths
}

func (snapshot worktreeSnapshot) prepareDirectory(root *os.Root, relative string) error {
	info, err := root.Lstat(relative)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect directory %q: %w", relative, err)
	}
	if err == nil && !info.IsDir() {
		if err := root.RemoveAll(relative); err != nil {
			return fmt.Errorf("replace directory %q: %w", relative, err)
		}
	}
	if err := root.MkdirAll(relative, 0o700); err != nil {
		return fmt.Errorf("create directory %q: %w", relative, err)
	}
	if err := chmodRootEntry(root, relative, snapshot.entries[relative].mode.Perm()|0o700); err != nil {
		return fmt.Errorf("prepare directory mode %q: %w", relative, err)
	}
	return nil
}

func (snapshot worktreeSnapshot) restoreFile(root *os.Root, relative string, entry worktreeEntry) error {
	if err := root.RemoveAll(relative); err != nil {
		return fmt.Errorf("remove changed path %q: %w", relative, err)
	}
	switch entry.kind {
	case worktreeRegularFile:
		if err := root.WriteFile(relative, entry.data, entry.mode.Perm()); err != nil {
			return fmt.Errorf("restore file %q: %w", relative, err)
		}
		if err := chmodRootEntry(root, relative, entry.mode.Perm()); err != nil {
			return fmt.Errorf("restore file mode %q: %w", relative, err)
		}
	case worktreeSymlink:
		if err := root.Symlink(entry.linkTarget, relative); err != nil {
			return fmt.Errorf("restore symlink %q: %w", relative, err)
		}
	default:
		return fmt.Errorf("%w: %q has snapshot kind %d", ErrUnsupportedWorktreeEntry, relative, entry.kind)
	}
	return nil
}

func pathDepth(path string) int {
	return strings.Count(path, "/")
}

func chmodRootEntry(root *os.Root, name string, mode fs.FileMode) (err error) {
	entry, err := root.Open(name)
	if err != nil {
		return fmt.Errorf("open for chmod: %w", err)
	}
	defer func() {
		if closeErr := entry.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close after chmod: %w", closeErr))
		}
	}()
	if err := entry.Chmod(mode); err != nil {
		return fmt.Errorf("chmod: %w", err)
	}
	return nil
}
