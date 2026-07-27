package apkrepository

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	apkversion "github.com/knqyf263/go-apk-version"
)

const (
	maxAPKMetadataSize = 16 << 20
	maxAPKDataSize     = int64(8 << 30)
	maxAPKEntryCount   = 100_000
	maxAPKEntrySize    = int64(2 << 30)
	maxAPKExpandedSize = maxAPKDataSize
	maxAPKPathLength   = 4096
	maxAPKLinkLength   = 4096
	maxAPKHeaderSize   = 64 << 10
)

var safePackageName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9+_.-]*$`)

type packageKey struct {
	architecture string
	name         string
}

type inspectedPackage struct {
	key      packageKey
	version  string
	path     string
	fileName string
	digest   string
}

func PackageSemanticDigest(packagePath string) (string, error) {
	metadata, err := inspectPackage(packagePath)
	if err != nil {
		return "", err
	}
	return metadata.digest, nil
}

func inspectPackage(packagePath string) (inspectedPackage, error) {
	file, err := os.Open(packagePath)
	if err != nil {
		return inspectedPackage{}, fmt.Errorf("open APK %q: %w", packagePath, err)
	}
	defer file.Close()
	reader := bufio.NewReader(file)
	firstSection, err := readAPKMetadataSection(reader)
	if err != nil {
		return inspectedPackage{}, fmt.Errorf("parse APK %q: %w", packagePath, err)
	}
	firstName, err := firstTarEntry(firstSection)
	if err != nil {
		return inspectedPackage{}, fmt.Errorf("parse APK %q: %w", packagePath, err)
	}
	controlSection := firstSection
	if strings.HasPrefix(firstName, ".SIGN.") {
		controlSection, err = readAPKMetadataSection(reader)
		if err != nil {
			return inspectedPackage{}, fmt.Errorf("parse APK %q control section: %w", packagePath, err)
		}
	}
	controlData, err := decompressAPKSection(controlSection, maxAPKMetadataSize)
	if err != nil {
		return inspectedPackage{}, fmt.Errorf("parse APK %q control section: %w", packagePath, err)
	}
	name, version, architecture, err := parsePackageInfo(controlData)
	if err != nil {
		return inspectedPackage{}, fmt.Errorf("parse APK %q .PKGINFO: %w", packagePath, err)
	}
	digest := sha256.New()
	_, _ = digest.Write([]byte("verity-apk-semantic-v1\x00"))
	_, _ = digest.Write(controlData)
	_, _ = digest.Write([]byte{0})
	if err := hashDataSection(reader, digest); err != nil {
		return inspectedPackage{}, fmt.Errorf("parse APK %q data section: %w", packagePath, err)
	}
	return inspectedPackage{
		key: packageKey{architecture: architecture, name: name}, version: version,
		path: packagePath, fileName: filepath.Base(packagePath), digest: "sha256:" + hex.EncodeToString(digest.Sum(nil)),
	}, nil
}

func readAPKMetadataSection(reader *bufio.Reader) ([]byte, error) {
	var compressed bytes.Buffer
	tee := &apkTeeByteReader{reader: reader, writer: &compressed}
	gzipReader, err := gzip.NewReader(tee)
	if err != nil {
		return nil, fmt.Errorf("open gzip stream: %w", err)
	}
	gzipReader.Multistream(false)
	limited := &io.LimitedReader{R: gzipReader, N: maxAPKMetadataSize + 1}
	written, copyErr := io.Copy(io.Discard, limited)
	closeErr := gzipReader.Close()
	if copyErr != nil {
		return nil, fmt.Errorf("read gzip stream: %w", copyErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close gzip stream: %w", closeErr)
	}
	if written > maxAPKMetadataSize {
		return nil, fmt.Errorf("%w: metadata stream exceeds %d bytes", errInvalidAPK, maxAPKMetadataSize)
	}
	return compressed.Bytes(), nil
}

func decompressAPKSection(compressed []byte, limit int64) ([]byte, error) {
	gzipReader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, fmt.Errorf("open gzip stream: %w", err)
	}
	defer gzipReader.Close()
	data, err := io.ReadAll(io.LimitReader(gzipReader, limit+1))
	if err != nil {
		return nil, fmt.Errorf("decompress gzip stream: %w", err)
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%w: decompressed stream exceeds %d bytes", errInvalidAPK, limit)
	}
	return data, nil
}

func firstTarEntry(compressed []byte) (string, error) {
	data, err := decompressAPKSection(compressed, maxAPKMetadataSize)
	if err != nil {
		return "", err
	}
	header, err := tar.NewReader(bytes.NewReader(data)).Next()
	if err != nil {
		return "", fmt.Errorf("read first tar entry: %w", err)
	}
	return header.Name, nil
}

func parsePackageInfo(controlData []byte) (name, version, architecture string, err error) {
	tarReader := tar.NewReader(bytes.NewReader(controlData))
	for {
		header, nextErr := tarReader.Next()
		if errors.Is(nextErr, io.EOF) {
			return "", "", "", fmt.Errorf("%w: .PKGINFO entry missing", errInvalidAPK)
		}
		if nextErr != nil {
			return "", "", "", fmt.Errorf("read control archive: %w", nextErr)
		}
		if header.Name != ".PKGINFO" {
			continue
		}
		return parsePackageInfoFields(tarReader)
	}
}

func parsePackageInfoFields(reader io.Reader) (name, version, architecture string, err error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), maxAPKMetadataSize)
	for scanner.Scan() {
		key, value, found := strings.Cut(scanner.Text(), "=")
		if !found {
			continue
		}
		switch strings.TrimSpace(key) {
		case "pkgname":
			name = strings.TrimSpace(value)
		case "pkgver":
			version = strings.TrimSpace(value)
		case "arch":
			architecture = strings.TrimSpace(value)
		}
	}
	if err := scanner.Err(); err != nil {
		return "", "", "", fmt.Errorf("read .PKGINFO: %w", err)
	}
	if !safePackageName.MatchString(name) {
		return "", "", "", fmt.Errorf("%w: invalid package name %q", errInvalidAPK, name)
	}
	if _, err := apkversion.NewVersion(version); err != nil {
		return "", "", "", fmt.Errorf("invalid package version %q: %w", version, err)
	}
	if !isSupportedArchitecture(architecture) {
		return "", "", "", fmt.Errorf("%w: %s", errUnsupportedArchitecture, architecture)
	}
	return name, version, architecture, nil
}

func hashDataSection(reader *bufio.Reader, digest hash.Hash) error {
	gzipReader, err := gzip.NewReader(reader)
	if err != nil {
		return fmt.Errorf("open gzip stream: %w", err)
	}
	gzipReader.Multistream(false)
	limited := &io.LimitedReader{R: gzipReader, N: maxAPKDataSize + 1}
	tarring := io.TeeReader(limited, digest)
	tarReader := tar.NewReader(tarring)
	validator := apkDataArchiveValidator{}
	for {
		header, nextErr := tarReader.Next()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			return fmt.Errorf("read data archive: %w", nextErr)
		}
		if err := validator.validate(header); err != nil {
			return err
		}
		read, err := io.Copy(io.Discard, tarReader)
		if err != nil {
			return fmt.Errorf("read data entry: %w", err)
		}
		if read != header.Size {
			return fmt.Errorf("%w: data entry %q read %d of %d bytes", errInvalidAPK, header.Name, read, header.Size)
		}
	}
	if _, err := io.Copy(apkZeroPaddingWriter{}, tarring); err != nil {
		return fmt.Errorf("finish data archive: %w", err)
	}
	if err := gzipReader.Close(); err != nil {
		return fmt.Errorf("close data stream: %w", err)
	}
	if limited.N == 0 {
		return fmt.Errorf("%w: data stream exceeds %d bytes", errInvalidAPK, maxAPKDataSize)
	}
	if _, err := reader.Peek(1); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("%w: unexpected bytes after data stream", errInvalidAPK)
		}
		return fmt.Errorf("read trailing APK bytes: %w", err)
	}
	return nil
}
