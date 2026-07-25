package sitepublication

import (
	"archive/tar"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/verity-org/verity/internal/ci/publication"
)

var archiveEpoch = time.Unix(0, 0).UTC()

func PackSite(siteDir, archivePath string) (publication.Digest, error) {
	snapshot, err := captureSiteSnapshot(siteDir)
	if err != nil {
		return "", err
	}
	defer snapshot.Close()
	return publishSiteSnapshot(snapshot, archivePath)
}

func publishSiteSnapshot(snapshot *siteSnapshot, archivePath string) (result publication.Digest, resultErr error) {
	parent, finalName, err := prepareArchiveTarget(snapshot, archivePath)
	if err != nil {
		return "", err
	}
	defer func() { resultErr = errors.Join(resultErr, parent.close()) }()
	name, err := randomArchiveName()
	if err != nil {
		return "", err
	}
	temporary, err := parent.createExclusive(name, 0o600)
	if err != nil {
		return "", err
	}
	committed := false
	defer func() {
		resultErr = errors.Join(resultErr, temporary.file.Close())
		if !committed {
			resultErr = errors.Join(resultErr, parent.remove(temporary.name))
		}
	}()
	result, err = packTrustedTree(snapshot.directory, temporary.file)
	if err != nil {
		return "", err
	}
	if err := temporary.file.Sync(); err != nil {
		return "", fmt.Errorf("sync site archive: %w", err)
	}
	if err := snapshot.revalidateSource(); err != nil {
		return "", err
	}
	if err := parent.commitExclusive(temporary, finalName); err != nil {
		return "", err
	}
	committed = true
	return result, nil
}

func prepareArchiveTarget(snapshot *siteSnapshot, archivePath string) (*secureRoot, string, error) {
	if snapshot == nil || snapshot.source == nil || archivePath == "" || filepath.Clean(archivePath) != archivePath {
		return nil, "", fmt.Errorf("%w: non-canonical archive path", ErrInvalidArchive)
	}
	absolute, err := filepath.Abs(archivePath)
	if err != nil {
		return nil, "", fmt.Errorf("%w: resolve archive path: %w", ErrInvalidArchive, err)
	}
	parent, err := openSecureRoot(filepath.Dir(absolute), ErrInvalidArchive)
	if err != nil {
		return nil, "", err
	}
	inside, err := snapshot.source.contains(parent)
	if err != nil {
		_ = parent.close()
		return nil, "", err
	}
	if inside {
		_ = parent.close()
		return nil, "", fmt.Errorf("%w: archive parent must be outside site directory", ErrInvalidArchive)
	}
	return parent, filepath.Base(absolute), nil
}

func packTrustedTree(root string, output io.Writer) (publication.Digest, error) {
	digest := sha256.New()
	writer := tar.NewWriter(io.MultiWriter(output, digest))
	files, err := listTreeFiles(root)
	if err != nil {
		return "", err
	}
	for _, file := range files {
		info, err := os.Lstat(file.path)
		if err != nil || !info.Mode().IsRegular() {
			return "", fmt.Errorf("%w: trusted snapshot file %q", ErrInvalidArchive, file.relative)
		}
		header := &tar.Header{
			Name: file.relative, Mode: int64(file.mode.Perm()), Size: info.Size(),
			Typeflag: tar.TypeReg, ModTime: archiveEpoch, Uid: 0, Gid: 0, Format: tar.FormatUSTAR,
		}
		if err := writer.WriteHeader(header); err != nil {
			return "", fmt.Errorf("write archive header %q: %w", file.relative, err)
		}
		input, err := os.Open(file.path)
		if err != nil {
			return "", fmt.Errorf("open snapshot file %q: %w", file.relative, err)
		}
		_, copyErr := io.Copy(writer, input)
		if err := errors.Join(copyErr, input.Close()); err != nil {
			return "", fmt.Errorf("write archive file %q: %w", file.relative, err)
		}
	}
	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("close tar writer: %w", err)
	}
	return publication.Digest("sha256:" + hex.EncodeToString(digest.Sum(nil))), nil
}

func randomArchiveName() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("create archive name: %w", err)
	}
	return ".site-artifact-" + hex.EncodeToString(value) + ".tar", nil
}
