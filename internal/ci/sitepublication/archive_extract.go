package sitepublication

import (
	"archive/tar"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func ExtractSiteArchive(reader io.Reader, outputDir string) (VerifiedSite, error) {
	if outputDir == "" || outputDir == "." || filepath.Clean(outputDir) == string(filepath.Separator) {
		return VerifiedSite{}, fmt.Errorf("%w: unsafe output directory", ErrInvalidArchive)
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return VerifiedSite{}, fmt.Errorf("create archive output: %w", err)
	}
	succeeded := false
	defer func() {
		if !succeeded {
			_ = os.RemoveAll(outputDir)
		}
	}()
	tarReader := tar.NewReader(reader)
	previous := ""
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return VerifiedSite{}, fmt.Errorf("%w: read tar: %w", ErrInvalidArchive, err)
		}
		previous, err = extractArchiveEntry(outputDir, previous, header, tarReader)
		if err != nil {
			return VerifiedSite{}, err
		}
	}
	verified, err := VerifySite(outputDir)
	if err != nil {
		return VerifiedSite{}, fmt.Errorf("%w: %w", ErrInvalidArchive, err)
	}
	succeeded = true
	return verified, nil
}

func extractArchiveEntry(outputDir, previous string, header *tar.Header, reader io.Reader) (string, error) {
	name, err := safeRelative(header.Name)
	if err != nil || name <= previous || !canonicalTarHeader(header) {
		return "", fmt.Errorf("%w: non-canonical entry %q", ErrInvalidArchive, header.Name)
	}
	destination := filepath.Join(outputDir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return "", fmt.Errorf("create extracted directory: %w", err)
	}
	file, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, os.FileMode(header.Mode))
	if err != nil {
		return "", fmt.Errorf("create extracted file %q: %w", name, err)
	}
	_, copyErr := io.Copy(file, reader)
	if err := errors.Join(copyErr, file.Close()); err != nil {
		return "", fmt.Errorf("extract file %q: %w", name, err)
	}
	return name, nil
}

func canonicalTarHeader(header *tar.Header) bool {
	mode := os.FileMode(header.Mode)
	return header.Typeflag == tar.TypeReg && (mode == 0o644 || mode == 0o755) &&
		header.Uid == 0 && header.Gid == 0 && header.Uname == "" && header.Gname == "" &&
		header.Format == tar.FormatUSTAR && header.ModTime.Equal(archiveEpoch)
}
