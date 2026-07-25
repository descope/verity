//go:build linux

package sitepublication

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

func (root *secureRoot) createExclusive(name string, mode os.FileMode) (*secureTempFile, error) {
	if filepath.Base(name) != name || name == "." || name == ".." {
		return nil, fmt.Errorf("%w: unsafe temporary archive name", ErrInvalidArchive)
	}
	fd, err := unix.Openat2(root.fd, name, &unix.OpenHow{
		Flags: uint64(unix.O_RDWR | unix.O_CREAT | unix.O_EXCL | unix.O_CLOEXEC | unix.O_NOFOLLOW),
		Mode:  uint64(mode.Perm()), Resolve: secureResolve | unix.RESOLVE_BENEATH,
	})
	if err != nil {
		return nil, fmt.Errorf("create secure site archive: %w", err)
	}
	return &secureTempFile{file: os.NewFile(uintptr(fd), name), fd: fd, name: name}, nil
}

func (root *secureRoot) commitExclusive(temporary *secureTempFile, finalName string) error {
	if temporary == nil || temporary.file == nil || filepath.Base(finalName) != finalName || finalName == "." || finalName == ".." {
		return fmt.Errorf("%w: unsafe archive commit", ErrInvalidArchive)
	}
	var opened unix.Stat_t
	if err := unix.Fstat(temporary.fd, &opened); err != nil {
		return fmt.Errorf("stat open site archive: %w", err)
	}
	var linked unix.Stat_t
	if err := unix.Fstatat(root.fd, temporary.name, &linked, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return fmt.Errorf("stat linked site archive: %w", err)
	}
	if !sameFileStat(&opened, &linked) || linked.Mode&unix.S_IFMT != unix.S_IFREG || linked.Nlink != 1 {
		return fmt.Errorf("%w: temporary archive path changed", ErrInvalidArchive)
	}
	if err := unix.Fchmod(temporary.fd, 0o644); err != nil {
		return fmt.Errorf("chmod site archive: %w", err)
	}
	if err := unix.Renameat2(root.fd, temporary.name, root.fd, finalName, unix.RENAME_NOREPLACE); err != nil {
		return fmt.Errorf("%w: publish site archive: %w", ErrInvalidArchive, err)
	}
	temporary.name = ""
	return nil
}

func (root *secureRoot) remove(name string) error {
	if name == "" {
		return nil
	}
	err := unix.Unlinkat(root.fd, name, 0)
	if errors.Is(err, unix.ENOENT) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("remove temporary site archive: %w", err)
	}
	return nil
}
