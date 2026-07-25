package sitepublication

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
	"strings"

	"github.com/verity-org/verity/internal/ci/publication"
)

const (
	maxSignerAPKMetadata = 16 << 20
	maxSignerAPKData     = int64(8 << 30)
)

var (
	errSignerAPKMetadataTooLarge = errors.New("APK metadata exceeds size limit")
	errSignerAPKMissingPackage   = errors.New("APK control section has no .PKGINFO")
	errSignerAPKDataTooLarge     = errors.New("APK data exceeds size limit")
)

func signerPackageSemanticDigest(path string) (publication.Digest, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open APK %q: %w", path, err)
	}
	defer file.Close()
	reader := bufio.NewReader(file)
	firstSection, err := readSignerAPKSection(reader)
	if err != nil {
		return "", err
	}
	firstName, err := signerAPKFirstEntry(firstSection)
	if err != nil {
		return "", err
	}
	controlSection := firstSection
	if strings.HasPrefix(firstName, ".SIGN.") {
		controlSection, err = readSignerAPKSection(reader)
		if err != nil {
			return "", err
		}
	}
	controlData, err := decompressSignerAPKSection(controlSection, maxSignerAPKMetadata)
	if err != nil {
		return "", err
	}
	if err := requireSignerPackageInfo(controlData); err != nil {
		return "", err
	}
	digest := sha256.New()
	_, _ = digest.Write([]byte("verity-apk-semantic-v1\x00"))
	_, _ = digest.Write(controlData)
	_, _ = digest.Write([]byte{0})
	if err := hashSignerAPKData(reader, digest); err != nil {
		return "", err
	}
	return publication.Digest("sha256:" + hex.EncodeToString(digest.Sum(nil))), nil
}

type signerAPKByteReader struct {
	reader *bufio.Reader
	writer io.Writer
}

func (reader *signerAPKByteReader) Read(p []byte) (int, error) {
	count, readErr := reader.reader.Read(p)
	if count > 0 {
		captureErr := writeSignerAPKCapture(reader.writer, p[:count])
		return count, errors.Join(readErr, captureErr)
	}
	return count, readErr
}

func (reader *signerAPKByteReader) ReadByte() (byte, error) {
	value, err := reader.reader.ReadByte()
	if err != nil {
		return value, err
	}
	return value, writeSignerAPKCapture(reader.writer, []byte{value})
}

func writeSignerAPKCapture(writer io.Writer, data []byte) error {
	written, err := writer.Write(data)
	if err != nil {
		return fmt.Errorf("capture APK gzip bytes: %w", err)
	}
	if written != len(data) {
		return fmt.Errorf("capture APK gzip bytes: %w", io.ErrShortWrite)
	}
	return nil
}

func readSignerAPKSection(reader *bufio.Reader) ([]byte, error) {
	var compressed bytes.Buffer
	tee := &signerAPKByteReader{reader: reader, writer: &compressed}
	gzipReader, err := gzip.NewReader(tee)
	if err != nil {
		return nil, fmt.Errorf("open APK gzip stream: %w", err)
	}
	gzipReader.Multistream(false)
	written, copyErr := io.Copy(io.Discard, io.LimitReader(gzipReader, maxSignerAPKMetadata+1))
	closeErr := gzipReader.Close()
	if err := errors.Join(copyErr, closeErr); err != nil {
		return nil, fmt.Errorf("read APK gzip stream: %w", err)
	}
	if written > maxSignerAPKMetadata {
		return nil, fmt.Errorf("%w: %d bytes", errSignerAPKMetadataTooLarge, maxSignerAPKMetadata)
	}
	return compressed.Bytes(), nil
}

func decompressSignerAPKSection(compressed []byte, limit int64) ([]byte, error) {
	gzipReader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, err
	}
	defer gzipReader.Close()
	data, err := io.ReadAll(io.LimitReader(gzipReader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%w: %d bytes", errSignerAPKMetadataTooLarge, limit)
	}
	return data, nil
}

func signerAPKFirstEntry(compressed []byte) (string, error) {
	data, err := decompressSignerAPKSection(compressed, maxSignerAPKMetadata)
	if err != nil {
		return "", err
	}
	header, err := tar.NewReader(bytes.NewReader(data)).Next()
	if err != nil {
		return "", fmt.Errorf("read APK tar entry: %w", err)
	}
	return header.Name, nil
}

func requireSignerPackageInfo(controlData []byte) error {
	reader := tar.NewReader(bytes.NewReader(controlData))
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return errSignerAPKMissingPackage
		}
		if err != nil {
			return err
		}
		if header.Name == ".PKGINFO" {
			return nil
		}
	}
}

func hashSignerAPKData(reader *bufio.Reader, digest hash.Hash) error {
	gzipReader, err := gzip.NewReader(reader)
	if err != nil {
		return fmt.Errorf("open APK data stream: %w", err)
	}
	gzipReader.Multistream(false)
	written, copyErr := io.Copy(io.Discard, io.LimitReader(io.TeeReader(gzipReader, digest), maxSignerAPKData+1))
	closeErr := gzipReader.Close()
	if err := errors.Join(copyErr, closeErr); err != nil {
		return fmt.Errorf("read APK data stream: %w", err)
	}
	if written > maxSignerAPKData {
		return fmt.Errorf("%w: %d bytes", errSignerAPKDataTooLarge, maxSignerAPKData)
	}
	if _, err := io.Copy(io.Discard, reader); err != nil {
		return fmt.Errorf("read trailing APK bytes: %w", err)
	}
	return nil
}
