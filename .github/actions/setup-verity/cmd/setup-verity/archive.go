package main

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	maxArchiveSize = 256 << 20
	maxBinarySize  = 200 << 20
	maxBuildJSON   = 64 << 10
	maxChecksum    = 80
)

type extractOptions struct {
	DownloadDirectory string
	ArtifactDirectory string
	ArtifactDigest    string
	Identity          artifactIdentity
}

func extractArtifact(options *extractOptions) (verified verifiedArtifact, err error) {
	if !options.Identity.valid() || !validArchiveDigest(options.ArtifactDigest) {
		return verifiedArtifact{}, untrusted("malformed extraction identity")
	}
	archivePath, err := singleArchive(options.DownloadDirectory)
	if err != nil {
		return verifiedArtifact{}, err
	}
	digest, err := digestPath(archivePath, maxArchiveSize)
	if err != nil {
		return verifiedArtifact{}, fmt.Errorf("digest artifact archive: %w", err)
	}
	if "sha256:"+digest != options.ArtifactDigest {
		return verifiedArtifact{}, untrusted("artifact archive digest")
	}
	if err := createArtifactDirectory(options.ArtifactDirectory); err != nil {
		return verifiedArtifact{}, err
	}
	defer func() {
		if err != nil {
			err = errorsJoin(err, os.RemoveAll(options.ArtifactDirectory))
		}
	}()
	if err := extractExactZip(archivePath, options.ArtifactDirectory); err != nil {
		return verifiedArtifact{}, err
	}
	return verifyArtifactDirectory(options.ArtifactDirectory, options.Identity)
}

func singleArchive(directory string) (string, error) {
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", untrusted("download directory")
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return "", fmt.Errorf("read download directory: %w", err)
	}
	if len(entries) != 1 {
		return "", untrusted("download file set")
	}
	path := filepath.Join(directory, entries[0].Name())
	entryInfo, err := os.Lstat(path)
	if err != nil || !entryInfo.Mode().IsRegular() || entryInfo.Size() > maxArchiveSize {
		return "", untrusted("download archive")
	}
	return path, nil
}

func createArtifactDirectory(directory string) error {
	parent, err := os.Lstat(filepath.Dir(directory))
	if err != nil || !parent.IsDir() || parent.Mode()&os.ModeSymlink != 0 {
		return untrusted("artifact parent directory")
	}
	if err := os.Mkdir(directory, 0o700); err != nil {
		return fmt.Errorf("create artifact directory: %w", err)
	}
	return nil
}

func extractExactZip(archivePath, directory string) (err error) {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return untrusted("invalid ZIP archive")
	}
	defer func() { err = errorsJoin(err, reader.Close()) }()
	if len(reader.File) != 3 {
		return untrusted("artifact archive file count")
	}
	wanted := map[string]uint64{binaryName: maxBinarySize, checksumName: maxChecksum, buildJSONName: maxBuildJSON}
	seen := make(map[string]struct{}, len(wanted))
	for _, file := range reader.File {
		limit, allowed := wanted[file.Name]
		if !allowed || filepath.Base(file.Name) != file.Name || strings.Contains(file.Name, `\`) {
			return untrusted("artifact archive path")
		}
		if _, duplicate := seen[file.Name]; duplicate {
			return untrusted("duplicate artifact archive entry")
		}
		mode := file.Mode()
		if !mode.IsRegular() || mode&os.ModeSymlink != 0 || file.UncompressedSize64 > limit {
			return untrusted("artifact archive entry type or size")
		}
		destination, ok := artifactEntryDestination(directory, file.Name)
		if !ok {
			return untrusted("artifact archive path")
		}
		if err := extractZipFile(file, destination, limit); err != nil {
			return err
		}
		seen[file.Name] = struct{}{}
	}
	if len(seen) != len(wanted) {
		return untrusted("artifact archive file set")
	}
	return nil
}

func artifactEntryDestination(directory, name string) (string, bool) {
	switch name {
	case binaryName:
		return filepath.Join(directory, binaryName), true
	case checksumName:
		return filepath.Join(directory, checksumName), true
	case buildJSONName:
		return filepath.Join(directory, buildJSONName), true
	default:
		return "", false
	}
}

func extractZipFile(source *zip.File, destination string, limit uint64) (err error) {
	reader, err := source.Open()
	if err != nil {
		return untrusted("open artifact archive entry")
	}
	defer func() { err = errorsJoin(err, reader.Close()) }()
	file, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create artifact entry: %w", err)
	}
	defer func() { err = errorsJoin(err, file.Close()) }()
	written, err := io.Copy(file, io.LimitReader(reader, int64(limit)+1))
	if err != nil {
		return fmt.Errorf("extract artifact entry: %w", err)
	}
	if written > int64(limit) {
		return untrusted("artifact archive entry expanded beyond limit")
	}
	return nil
}

func validArchiveDigest(value string) bool {
	return strings.HasPrefix(value, "sha256:") && lowerHex(strings.TrimPrefix(value, "sha256:"), 64)
}
