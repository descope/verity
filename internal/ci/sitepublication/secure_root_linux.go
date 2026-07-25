//go:build linux

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

	"golang.org/x/sys/unix"
)

const secureResolve = unix.RESOLVE_NO_MAGICLINKS | unix.RESOLVE_NO_SYMLINKS

func openSecureRoot(path string, invalid error) (*secureRoot, error) {
	if path == "" || filepath.Clean(path) != path {
		return nil, fmt.Errorf("%w: non-canonical directory %q", invalid, path)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("%w: absolute directory %q: %w", invalid, path, err)
	}
	fd, err := unix.Openat2(unix.AT_FDCWD, absolute, &unix.OpenHow{
		Flags: uint64(unix.O_RDONLY | unix.O_DIRECTORY | unix.O_CLOEXEC), Resolve: secureResolve,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: open directory without links %q: %w", invalid, path, err)
	}
	return &secureRoot{file: os.NewFile(uintptr(fd), absolute), fd: fd}, nil
}

func openSecureFilePath(path string, invalid error) (*os.File, error) {
	if path == "" || filepath.Clean(path) != path {
		return nil, fmt.Errorf("%w: non-canonical file %q", invalid, path)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("%w: absolute file %q: %w", invalid, path, err)
	}
	parent, err := openSecureRoot(filepath.Dir(absolute), invalid)
	if err != nil {
		return nil, err
	}
	defer parent.close()
	file, stat, err := parent.openChild(filepath.Base(absolute))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", invalid, err)
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Nlink != 1 {
		_ = file.Close()
		return nil, fmt.Errorf("%w: archive is not one unlinked regular file", invalid)
	}
	return file, nil
}

func (root *secureRoot) scan(destination string) ([]treeFingerprint, error) {
	start, _, err := root.openChild(".")
	if err != nil {
		return nil, err
	}
	defer start.Close()
	scanner := secureScanner{destination: destination, fingerprints: make([]treeFingerprint, 0)}
	if err := scanner.scanDirectory(start, ""); err != nil {
		return nil, err
	}
	return scanner.fingerprints, nil
}

type secureScanner struct {
	destination  string
	fingerprints []treeFingerprint
}

func (scanner *secureScanner) scanDirectory(directory *os.File, prefix string) error {
	entries, err := directory.ReadDir(-1)
	if err != nil {
		return fmt.Errorf("%w: read secure site directory %q: %w", ErrInvalidAssembly, prefix, err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if strings.Contains(entry.Name(), "/") || entry.Name() == "." || entry.Name() == ".." {
			return fmt.Errorf("%w: unsafe site entry %q", ErrInvalidAssembly, entry.Name())
		}
		relative := entry.Name()
		if prefix != "" {
			relative = prefix + "/" + entry.Name()
		}
		file, stat, err := openSecureChild(int(directory.Fd()), entry.Name())
		if err != nil {
			return fmt.Errorf("%w: open site entry %q: %w", ErrInvalidAssembly, relative, err)
		}
		switch stat.Mode & unix.S_IFMT {
		case unix.S_IFDIR:
			if err := scanner.scanDirectory(file, relative); err != nil {
				_ = file.Close()
				return err
			}
		case unix.S_IFREG:
			fingerprint, err := scanner.fingerprintFile(file, &stat, relative)
			if err != nil {
				_ = file.Close()
				return err
			}
			scanner.fingerprints = append(scanner.fingerprints, fingerprint)
		default:
			_ = file.Close()
			return fmt.Errorf("%w: unsupported or linked site entry %q", ErrInvalidAssembly, relative)
		}
		if err := file.Close(); err != nil {
			return fmt.Errorf("close site entry %q: %w", relative, err)
		}
	}
	return nil
}

func (scanner *secureScanner) fingerprintFile(input *os.File, before *unix.Stat_t, relative string) (treeFingerprint, error) {
	if before.Nlink != 1 {
		return treeFingerprint{}, fmt.Errorf("%w: hard-linked site file %q", ErrInvalidAssembly, relative)
	}
	digest := sha256.New()
	var writer io.Writer = digest
	var output *os.File
	if scanner.destination != "" {
		path := filepath.Join(scanner.destination, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return treeFingerprint{}, fmt.Errorf("create snapshot directory: %w", err)
		}
		var err error
		output, err = os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, normalizedMode(os.FileMode(before.Mode)))
		if err != nil {
			return treeFingerprint{}, fmt.Errorf("create snapshot file %q: %w", relative, err)
		}
		writer = io.MultiWriter(digest, output)
	}
	_, copyErr := io.Copy(writer, input)
	var closeErr error
	if output != nil {
		closeErr = output.Close()
	}
	if err := errors.Join(copyErr, closeErr); err != nil {
		return treeFingerprint{}, fmt.Errorf("snapshot site file %q: %w", relative, err)
	}
	var after unix.Stat_t
	if err := unix.Fstat(int(input.Fd()), &after); err != nil || !stableFileStat(before, &after) {
		return treeFingerprint{}, fmt.Errorf("%w: site file changed while reading %q", ErrUndeclaredMutation, relative)
	}
	return treeFingerprint{
		path: relative, digest: "sha256:" + hex.EncodeToString(digest.Sum(nil)),
		mode: normalizedMode(os.FileMode(before.Mode)),
	}, nil
}

func stableFileStat(before, after *unix.Stat_t) bool {
	return before.Dev == after.Dev && before.Ino == after.Ino && before.Mode == after.Mode && before.Nlink == after.Nlink &&
		before.Size == after.Size && before.Mtim == after.Mtim
}

func openatNoLinks(directoryFD int, path string) (int, error) {
	return unix.Openat2(directoryFD, path, &unix.OpenHow{
		Flags:   uint64(unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW | unix.O_NONBLOCK),
		Resolve: secureResolve | unix.RESOLVE_BENEATH,
	})
}

func (root *secureRoot) openChild(path string) (*os.File, unix.Stat_t, error) {
	return openSecureChild(root.fd, path)
}

func openSecureChild(directoryFD int, path string) (*os.File, unix.Stat_t, error) {
	fd, err := openatNoLinks(directoryFD, path)
	if err != nil {
		return nil, unix.Stat_t{}, fmt.Errorf("open secure path %q: %w", path, err)
	}
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		_ = unix.Close(fd)
		return nil, unix.Stat_t{}, fmt.Errorf("stat secure path %q: %w", path, err)
	}
	return os.NewFile(uintptr(fd), path), stat, nil
}

func (root *secureRoot) contains(candidate *secureRoot) (bool, error) {
	target, err := root.stat()
	if err != nil {
		return false, err
	}
	currentFD, err := openatNoLinks(candidate.fd, ".")
	if err != nil {
		return false, err
	}
	defer func() { _ = unix.Close(currentFD) }()
	for range 1024 {
		var currentStat unix.Stat_t
		if err := unix.Fstat(currentFD, &currentStat); err != nil {
			return false, fmt.Errorf("stat archive ancestor: %w", err)
		}
		if sameFileStat(&target, &currentStat) {
			return true, nil
		}
		parentFD, err := unix.Openat2(currentFD, "..", &unix.OpenHow{
			Flags: uint64(unix.O_RDONLY | unix.O_DIRECTORY | unix.O_CLOEXEC), Resolve: secureResolve,
		})
		if err != nil {
			return false, fmt.Errorf("open archive ancestor: %w", err)
		}
		var parentStat unix.Stat_t
		if err := unix.Fstat(parentFD, &parentStat); err != nil {
			_ = unix.Close(parentFD)
			return false, fmt.Errorf("stat archive parent: %w", err)
		}
		if sameFileStat(&currentStat, &parentStat) {
			_ = unix.Close(parentFD)
			return false, nil
		}
		_ = unix.Close(currentFD)
		currentFD = parentFD
	}
	return false, fmt.Errorf("%w: archive ancestry depth", ErrInvalidArchive)
}

func (root *secureRoot) stat() (unix.Stat_t, error) {
	var stat unix.Stat_t
	err := unix.Fstat(root.fd, &stat)
	return stat, err
}

func sameFileStat(left, right *unix.Stat_t) bool {
	return left.Dev == right.Dev && left.Ino == right.Ino
}
