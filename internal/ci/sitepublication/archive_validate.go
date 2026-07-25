package sitepublication

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/verity-org/verity/internal/ci/publication"
)

func ValidateArchive(path string, artifactDigest, manifestDigest publication.Digest) (VerifiedSite, error) {
	file, err := openSecureFilePath(path, ErrInvalidArchive)
	if err != nil {
		return VerifiedSite{}, err
	}
	defer file.Close()
	actualDigest, err := digestOpenFile(file)
	if err != nil {
		return VerifiedSite{}, err
	}
	if actualDigest != artifactDigest {
		return VerifiedSite{}, fmt.Errorf("%w: expected %s, got %s", ErrArtifactTampered, artifactDigest, actualDigest)
	}
	output, err := os.MkdirTemp("", "verity-site-archive-verify-")
	if err != nil {
		return VerifiedSite{}, fmt.Errorf("create archive verification directory: %w", err)
	}
	_ = os.Remove(output)
	defer os.RemoveAll(output)
	verified, err := ExtractSiteArchive(file, output)
	if err != nil {
		return VerifiedSite{}, err
	}
	if verified.ManifestDigest != manifestDigest {
		return VerifiedSite{}, fmt.Errorf("%w: manifest digest", ErrArtifactTampered)
	}
	repackedPath := output + ".tar"
	defer os.Remove(repackedPath)
	repackedDigest, err := PackSite(output, repackedPath)
	if err != nil {
		return VerifiedSite{}, err
	}
	if repackedDigest != actualDigest {
		return VerifiedSite{}, fmt.Errorf("%w: archive bytes are not canonical", ErrInvalidArchive)
	}
	return verified, nil
}

func digestOpenFile(file *os.File) (publication.Digest, error) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", fmt.Errorf("seek site archive: %w", err)
	}
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return "", fmt.Errorf("hash site archive: %w", err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", fmt.Errorf("rewind site archive: %w", err)
	}
	return publication.Digest("sha256:" + hex.EncodeToString(digest.Sum(nil))), nil
}

func removeSecureFilePath(path string) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve archive cleanup path: %w", err)
	}
	parent, err := openSecureRoot(filepath.Dir(absolute), ErrInvalidArchive)
	if err != nil {
		return err
	}
	defer parent.close()
	return parent.remove(filepath.Base(absolute))
}
