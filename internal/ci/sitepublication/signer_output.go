package sitepublication

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/verity-org/verity/internal/ci/publication"
)

func signerRepositoryDigest(root string) (publication.Digest, error) {
	records, err := scanSignerDirectory(root)
	if err != nil {
		return "", err
	}
	managed := make([]signerPathRecord, 0, len(records))
	for _, record := range records {
		include, classifyErr := classifySignerRepositoryPath(record.relative)
		if classifyErr != nil {
			return "", classifyErr
		}
		if include {
			managed = append(managed, record)
		}
	}
	sort.Slice(managed, func(i, j int) bool { return managed[i].relative < managed[j].relative })
	digest := sha256.New()
	for _, record := range managed {
		if err := writeSignerDigestField(digest, record.relative); err != nil {
			return "", err
		}
		file, err := os.Open(record.path)
		if err != nil {
			return "", fmt.Errorf("open signer output %q: %w", record.path, err)
		}
		info, statErr := file.Stat()
		if statErr == nil {
			statErr = binary.Write(digest, binary.BigEndian, uint64(info.Size()))
		}
		_, copyErr := io.Copy(digest, file)
		closeErr := file.Close()
		if err := errors.Join(statErr, copyErr, closeErr); err != nil {
			return "", fmt.Errorf("digest signer output %q: %w", record.path, err)
		}
	}
	return publication.Digest("sha256:" + hex.EncodeToString(digest.Sum(nil))), nil
}

func classifySignerRepositoryPath(relative string) (bool, error) {
	parts := strings.Split(relative, "/")
	switch len(parts) {
	case 1:
		if parts[0] == "repository-format" || strings.HasSuffix(parts[0], ".rsa.pub") {
			return true, nil
		}
		if parts[0] == ".no-apks-found" {
			return false, nil
		}
	case 2:
		architectureAllowed := parts[0] == "x86_64" || parts[0] == "aarch64"
		artifactAllowed := parts[1] == "APKINDEX.tar.gz" || strings.HasSuffix(parts[1], ".apk")
		if architectureAllowed && artifactAllowed {
			return true, nil
		}
	}
	return false, fmt.Errorf("%w: unexpected signed repository path %q", ErrSignerExecution, relative)
}

func writeSignerDigestField(digest hash.Hash, value string) error {
	if err := binary.Write(digest, binary.BigEndian, uint32(len(value))); err != nil {
		return err
	}
	_, err := io.WriteString(digest, filepath.ToSlash(value))
	return err
}
