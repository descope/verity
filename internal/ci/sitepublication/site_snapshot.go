package sitepublication

import (
	"errors"
	"fmt"
	"os"
	"slices"
)

type treeFingerprint struct {
	path   string
	digest string
	mode   os.FileMode
}

type secureRoot struct {
	file *os.File
	fd   int
}

type secureTempFile struct {
	file *os.File
	fd   int
	name string
}

type siteSnapshot struct {
	source      *secureRoot
	directory   string
	fingerprint []treeFingerprint
	verified    VerifiedSite
}

func captureSiteSnapshot(siteDir string) (*siteSnapshot, error) {
	source, err := openSecureRoot(siteDir, ErrInvalidAssembly)
	if err != nil {
		return nil, err
	}
	directory, err := os.MkdirTemp("", "verity-site-snapshot-")
	if err != nil {
		_ = source.close()
		return nil, fmt.Errorf("create secure site snapshot: %w", err)
	}
	snapshot := &siteSnapshot{source: source, directory: directory}
	snapshot.fingerprint, err = source.scan(directory)
	if err != nil {
		_ = snapshot.Close()
		return nil, err
	}
	snapshot.verified, err = verifySiteTree(directory)
	if err != nil {
		_ = snapshot.Close()
		return nil, err
	}
	return snapshot, nil
}

func (snapshot *siteSnapshot) Close() error {
	if snapshot == nil {
		return nil
	}
	var closeErr error
	if snapshot.source != nil {
		closeErr = snapshot.source.close()
		snapshot.source = nil
	}
	removeErr := os.RemoveAll(snapshot.directory)
	snapshot.directory = ""
	return errors.Join(closeErr, removeErr)
}

func (snapshot *siteSnapshot) revalidateSource() error {
	actual, err := snapshot.source.scan("")
	if err != nil {
		return err
	}
	if !slices.Equal(snapshot.fingerprint, actual) {
		return fmt.Errorf("%w: source changed after secure snapshot", ErrUndeclaredMutation)
	}
	return nil
}

func (root *secureRoot) close() error {
	if root == nil || root.file == nil {
		return nil
	}
	err := root.file.Close()
	root.file = nil
	root.fd = -1
	return err
}
