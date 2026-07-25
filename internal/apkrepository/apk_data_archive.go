package apkrepository

import (
	"archive/tar"
	"fmt"
	"io"
	"path"
	"strings"
	"unicode/utf8"
)

const (
	legacyTarRegularType byte = 0
	paxXattrNamespace         = "SCHILY.xattr."
)

type apkDataArchiveValidator struct {
	paths        map[string]struct{}
	entryCount   int
	expandedSize int64
}

func (validator *apkDataArchiveValidator) validate(header *tar.Header) error {
	if err := validator.validateHeader(header); err != nil {
		return err
	}
	normalized, err := canonicalAPKDataPath(header.Name, header.Typeflag == tar.TypeDir)
	if err != nil {
		return err
	}
	if err := validateAPKDataEntry(header); err != nil {
		return err
	}
	return validator.recordEntry(normalized, header.Size)
}

func (validator *apkDataArchiveValidator) validateHeader(header *tar.Header) error {
	if header == nil {
		return fmt.Errorf("%w: missing data archive header", errInvalidAPK)
	}
	validator.entryCount++
	if validator.entryCount > maxAPKEntryCount {
		return fmt.Errorf("%w: data archive exceeds %d entries", errInvalidAPK, maxAPKEntryCount)
	}
	if len(header.Name) > maxAPKPathLength {
		return fmt.Errorf("%w: data path exceeds %d bytes", errInvalidAPK, maxAPKPathLength)
	}
	if len(header.Linkname) > maxAPKLinkLength {
		return fmt.Errorf("%w: data link exceeds %d bytes", errInvalidAPK, maxAPKLinkLength)
	}
	if apkHeaderSize(header) > maxAPKHeaderSize {
		return fmt.Errorf("%w: data header exceeds %d bytes", errInvalidAPK, maxAPKHeaderSize)
	}
	return nil
}

func validateAPKDataEntry(header *tar.Header) error {
	if header.Linkname != "" && (!utf8.ValidString(header.Linkname) || strings.ContainsAny(header.Linkname, "\\\x00")) {
		return fmt.Errorf("%w: ambiguous data link %q", errInvalidAPK, header.Linkname)
	}
	switch header.Typeflag {
	case tar.TypeReg, legacyTarRegularType:
		if header.Linkname != "" {
			return fmt.Errorf("%w: regular file %q has a link target", errInvalidAPK, header.Name)
		}
	case tar.TypeDir:
		if header.Size != 0 || header.Linkname != "" {
			return fmt.Errorf("%w: unsafe directory entry %q", errInvalidAPK, header.Name)
		}
	default:
		return fmt.Errorf("%w: unsupported data entry type %q for %q", errInvalidAPK, header.Typeflag, header.Name)
	}
	if header.Size < 0 || header.Size > maxAPKEntrySize {
		return fmt.Errorf("%w: data entry %q exceeds %d bytes", errInvalidAPK, header.Name, maxAPKEntrySize)
	}
	return nil
}

func (validator *apkDataArchiveValidator) recordEntry(normalized string, size int64) error {
	if size > maxAPKExpandedSize-validator.expandedSize {
		return fmt.Errorf("%w: data archive exceeds %d expanded bytes", errInvalidAPK, maxAPKExpandedSize)
	}
	if validator.paths == nil {
		validator.paths = make(map[string]struct{})
	}
	if _, exists := validator.paths[normalized]; exists {
		return fmt.Errorf("%w: duplicate data path %q", errInvalidAPK, normalized)
	}
	validator.paths[normalized] = struct{}{}
	validator.expandedSize += size
	return nil
}

func canonicalAPKDataPath(name string, directory bool) (string, error) {
	if name == "" || !utf8.ValidString(name) || strings.ContainsAny(name, "\\\x00") || path.IsAbs(name) {
		return "", fmt.Errorf("%w: invalid data path %q", errInvalidAPK, name)
	}
	trimmed := name
	if directory {
		trimmed = strings.TrimSuffix(name, "/")
	}
	cleaned := path.Clean(trimmed)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("%w: escaping data path %q", errInvalidAPK, name)
	}
	if name != cleaned && (!directory || name != cleaned+"/") {
		return "", fmt.Errorf("%w: noncanonical data path %q", errInvalidAPK, name)
	}
	return cleaned, nil
}

func apkHeaderSize(header *tar.Header) int {
	size := len(header.Name) + len(header.Linkname) + len(header.Uname) + len(header.Gname)
	for key, value := range header.PAXRecords {
		size += len(key) + len(value)
		if value != "" && strings.HasPrefix(key, paxXattrNamespace) {
			size += len(strings.TrimPrefix(key, paxXattrNamespace)) + len(value)
		}
		if size > maxAPKHeaderSize {
			return size
		}
	}
	return size
}

type apkZeroPaddingWriter struct{}

func (apkZeroPaddingWriter) Write(data []byte) (int, error) {
	for _, value := range data {
		if value != 0 {
			return 0, fmt.Errorf("%w: non-zero bytes after data archive EOF", errInvalidAPK)
		}
	}
	return len(data), nil
}

type apkTeeByteReader struct {
	reader interface {
		io.Reader
		io.ByteReader
	}
	writer interface {
		io.Writer
		io.ByteWriter
	}
}

func (reader *apkTeeByteReader) Read(buffer []byte) (int, error) {
	read, err := reader.reader.Read(buffer)
	if read > 0 {
		_, writeErr := reader.writer.Write(buffer[:read])
		if writeErr != nil {
			return read, writeErr
		}
	}
	return read, err
}

func (reader *apkTeeByteReader) ReadByte() (byte, error) {
	value, err := reader.reader.ReadByte()
	if err == nil {
		err = reader.writer.WriteByte(value)
	}
	return value, err
}
