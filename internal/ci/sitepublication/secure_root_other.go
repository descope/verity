//go:build !linux

package sitepublication

import (
	"fmt"
	"os"
)

func openSecureRoot(path string, invalid error) (*secureRoot, error) {
	return nil, fmt.Errorf("%w: descriptor-safe site access requires Linux: %s", invalid, path)
}

func openSecureFilePath(path string, invalid error) (*os.File, error) {
	return nil, fmt.Errorf("%w: descriptor-safe file access requires Linux: %s", invalid, path)
}

func (root *secureRoot) scan(string) ([]treeFingerprint, error) {
	return nil, fmt.Errorf("%w: descriptor-safe site access requires Linux", ErrInvalidAssembly)
}

func (root *secureRoot) contains(*secureRoot) (bool, error) {
	return false, fmt.Errorf("%w: descriptor-safe site access requires Linux", ErrInvalidArchive)
}

func (root *secureRoot) createExclusive(string, os.FileMode) (*secureTempFile, error) {
	return nil, fmt.Errorf("%w: descriptor-safe archive creation requires Linux", ErrInvalidArchive)
}

func (root *secureRoot) commitExclusive(*secureTempFile, string) error {
	return fmt.Errorf("%w: descriptor-safe archive creation requires Linux", ErrInvalidArchive)
}

func (root *secureRoot) remove(string) error {
	return fmt.Errorf("%w: descriptor-safe archive creation requires Linux", ErrInvalidArchive)
}
