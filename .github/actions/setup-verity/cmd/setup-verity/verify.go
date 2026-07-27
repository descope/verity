package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/verity-org/verity/internal/buildmetadata"
)

type verifiedArtifact struct {
	BinaryPath   string
	BinaryDigest string
	Metadata     buildmetadata.Info
}

func verifyArtifactDirectory(directory string, identity artifactIdentity) (verifiedArtifact, error) {
	if !identity.valid() {
		return verifiedArtifact{}, untrusted("malformed artifact identity")
	}
	if err := exactArtifactFiles(directory); err != nil {
		return verifiedArtifact{}, err
	}
	metadataData, err := os.ReadFile(filepath.Join(directory, buildJSONName))
	if err != nil || len(metadataData) > maxBuildJSON {
		return verifiedArtifact{}, untrusted("read build metadata")
	}
	var metadata buildmetadata.Info
	decoder := json.NewDecoder(bytes.NewReader(metadataData))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&metadata); err != nil {
		return verifiedArtifact{}, untrusted("decode build metadata")
	}
	canonical, err := buildmetadata.MarshalInfo(metadata)
	if err != nil || !bytes.Equal(canonical, metadataData) || !validBuildMetadata(&metadata, identity) {
		return verifiedArtifact{}, untrusted("canonical build metadata")
	}
	binaryPath := filepath.Join(directory, binaryName)
	digest, err := digestPath(binaryPath, maxBinarySize)
	if err != nil {
		return verifiedArtifact{}, fmt.Errorf("digest Verity binary: %w", err)
	}
	checksum, err := os.ReadFile(filepath.Join(directory, checksumName))
	if err != nil || !bytes.Equal(checksum, []byte(digest+"  "+binaryName+"\n")) {
		return verifiedArtifact{}, untrusted("checksum record")
	}
	return verifiedArtifact{BinaryPath: binaryPath, BinaryDigest: digest, Metadata: metadata}, nil
}

func exactArtifactFiles(directory string) error {
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return untrusted("artifact directory")
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return fmt.Errorf("read artifact directory: %w", err)
	}
	if len(entries) != 3 {
		return untrusted("artifact file count")
	}
	wanted := []string{buildJSONName, checksumName, binaryName}
	for _, entry := range entries {
		if !slices.Contains(wanted, entry.Name()) {
			return untrusted("artifact file set")
		}
		entryInfo, err := os.Lstat(filepath.Join(directory, entry.Name()))
		if err != nil || !entryInfo.Mode().IsRegular() {
			return untrusted("artifact entry type")
		}
	}
	return nil
}

func validBuildMetadata(info *buildmetadata.Info, identity artifactIdentity) bool {
	return info.Version == buildVersion && info.SourceSHA == identity.SourceSHA &&
		info.BuildKey == identity.BuildKey && strings.HasPrefix(info.GoVersion, "go") &&
		info.GOOS == "linux" && info.GOARCH == "amd64" && info.CGOEnabled == "0" &&
		slices.Equal(info.BuildFlags, runtimeBuildFlags) && info.Dirty != nil && !*info.Dirty &&
		info.VCSStatus == buildmetadata.CleanVCSStatus && info.BuildStatus == buildmetadata.BuiltStatus
}

func digestPath(path string, limit int64) (digest string, err error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { err = errorsJoin(err, file.Close()) }()
	hasher := sha256.New()
	written, err := io.Copy(hasher, io.LimitReader(file, limit+1))
	if err != nil {
		return "", err
	}
	if written > limit {
		return "", untrusted("file size limit")
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func errorsJoin(primary, cleanup error) error {
	return errors.Join(primary, cleanup)
}
