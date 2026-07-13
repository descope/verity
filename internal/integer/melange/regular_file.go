package melange

import (
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

var replaceRegularFileAfterOpenParent func()

func replaceRegularFile(rootPath, path string, data []byte, mode os.FileMode, notRegular error) error {
	root, parentRelative, err := ensureManagedDirectory(rootPath, filepath.Dir(path))
	if err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	defer root.Close()
	parent, err := root.OpenRoot(parentRelative)
	if err != nil {
		return fmt.Errorf("open %s: %w", filepath.Dir(path), err)
	}
	defer parent.Close()
	if replaceRegularFileAfterOpenParent != nil {
		replaceRegularFileAfterOpenParent()
	}
	name := filepath.Base(path)
	if info, err := parent.Lstat(name); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%w: %s", notRegular, path)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect %s: %w", path, err)
	}
	temporaryName := ".verity-regular-" + rand.Text()
	temporary, err := parent.OpenFile(temporaryName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return fmt.Errorf("create temporary %s: %w", path, err)
	}
	if _, err := temporary.Write(data); err != nil {
		closeErr := temporary.Close()
		return errors.Join(fmt.Errorf("write temporary %s: %w", path, err), closeErr, removeTemporaryRegularFile(parent, temporaryName, path))
	}
	if err := temporary.Chmod(mode); err != nil {
		closeErr := temporary.Close()
		return errors.Join(fmt.Errorf("set mode on temporary %s: %w", path, err), closeErr, removeTemporaryRegularFile(parent, temporaryName, path))
	}
	if err := temporary.Close(); err != nil {
		return errors.Join(fmt.Errorf("close temporary %s: %w", path, err), removeTemporaryRegularFile(parent, temporaryName, path))
	}
	if err := parent.Rename(temporaryName, name); err != nil {
		return errors.Join(fmt.Errorf("replace %s: %w", path, err), removeTemporaryRegularFile(parent, temporaryName, path))
	}
	return nil
}

func removeTemporaryRegularFile(parent *os.Root, name, path string) error {
	if err := parent.Remove(name); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove temporary %s: %w", path, err)
	}
	return nil
}
