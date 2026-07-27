package repositoryops

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

var ErrUnsupportedGitState = errors.New("unsupported Git metadata state")

type gitFileSnapshot struct {
	path   string
	exists bool
	mode   fs.FileMode
	data   []byte
}

type gitFileSnapshotRequest struct {
	path     string
	label    string
	required bool
}

func captureGitFile(request gitFileSnapshotRequest) (gitFileSnapshot, error) {
	if request.path == "" || containsControl(request.path) || !filepath.IsAbs(request.path) {
		return gitFileSnapshot{}, fmt.Errorf("%w: %s path %q", ErrUnsupportedGitState, request.label, request.path)
	}
	path := filepath.Clean(request.path)
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) && !request.required {
		return gitFileSnapshot{path: path}, nil
	}
	if err != nil {
		return gitFileSnapshot{}, fmt.Errorf("inspect %s %q: %w", request.label, path, err)
	}
	if !info.Mode().IsRegular() {
		return gitFileSnapshot{}, fmt.Errorf("%w: %s %q has mode %s", ErrUnsupportedGitState, request.label, path, info.Mode())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return gitFileSnapshot{}, fmt.Errorf("read %s %q: %w", request.label, path, err)
	}
	return gitFileSnapshot{path: path, exists: true, mode: info.Mode(), data: data}, nil
}

func (snapshot gitFileSnapshot) restore() (err error) {
	if !snapshot.exists {
		return removeGitFile(snapshot.path)
	}
	if err := snapshot.validateRestoreTarget(); err != nil {
		return err
	}
	return snapshot.replaceAtomically()
}

func (snapshot gitFileSnapshot) validateRestoreTarget() error {
	current, err := os.Lstat(snapshot.path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect current Git metadata %q: %w", snapshot.path, err)
	}
	if err == nil && current.IsDir() {
		return fmt.Errorf("%w: Git metadata %q became a directory", ErrUnsupportedGitState, snapshot.path)
	}
	return nil
}

func (snapshot gitFileSnapshot) replaceAtomically() (err error) {
	temporary, err := os.CreateTemp(filepath.Dir(snapshot.path), ".verity-git-restore-*")
	if err != nil {
		return fmt.Errorf("create Git metadata restore file: %w", err)
	}
	temporaryPath := temporary.Name()
	closed := false
	defer func() {
		if !closed {
			if closeErr := temporary.Close(); closeErr != nil {
				err = errors.Join(err, fmt.Errorf("close failed Git metadata restore file: %w", closeErr))
			}
		}
		if removeErr := os.Remove(temporaryPath); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			err = errors.Join(err, fmt.Errorf("remove Git metadata restore file: %w", removeErr))
		}
	}()
	if err := temporary.Chmod(snapshot.mode.Perm()); err != nil {
		return fmt.Errorf("set Git metadata restore mode: %w", err)
	}
	if _, err := temporary.Write(snapshot.data); err != nil {
		return fmt.Errorf("write Git metadata restore file: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync Git metadata restore file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close Git metadata restore file: %w", err)
	}
	closed = true
	if err := os.Rename(temporaryPath, snapshot.path); err != nil {
		return fmt.Errorf("replace Git metadata %q: %w", snapshot.path, err)
	}
	return nil
}

func removeGitFile(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect transient Git metadata %q: %w", path, err)
	}
	if info.IsDir() {
		return fmt.Errorf("%w: transient Git metadata %q is a directory", ErrUnsupportedGitState, path)
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("remove transient Git metadata %q: %w", path, err)
	}
	return nil
}
