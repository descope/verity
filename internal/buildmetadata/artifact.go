package buildmetadata

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
)

const (
	// BinaryName is the only executable accepted in a Verity artifact.
	BinaryName = "verity"
	// ChecksumName contains the canonical SHA-256 checksum record.
	ChecksumName = "verity.sha256"
	// ManifestName contains the canonical artifact identity manifest.
	ManifestName = "build-metadata.json"
)

// ErrInvalidArtifact reports a payload that cannot be trusted.
var ErrInvalidArtifact = errors.New("invalid build artifact")

// PackageOptions identifies the binary and trusted build inputs to package.
type PackageOptions struct {
	Directory string
	SourceSHA string
	BuildKey  string
	GoVersion string
}

// VerifyOptions identifies the artifact and trusted identity to verify.
type VerifyOptions struct {
	Directory string
	SourceSHA string
	BuildKey  string
}

// Manifest is the canonical identity record stored beside the binary.
type Manifest struct {
	BinaryName   string `json:"binary_name"`
	BinarySHA256 string `json:"binary_sha256"`
	SourceSHA    string `json:"source_sha"`
	BuildKey     string `json:"build_key"`
	GoVersion    string `json:"go_version"`
}

// VerifiedArtifact is returned only after every artifact invariant passes.
type VerifiedArtifact struct {
	Manifest   Manifest
	BinaryPath string
}

// PackageArtifact writes the canonical three-file artifact payload.
func PackageArtifact(options PackageOptions) (Manifest, error) {
	if err := validateArtifactIdentity(options.SourceSHA, options.BuildKey); err != nil {
		return Manifest{}, err
	}
	if options.GoVersion == "" {
		return Manifest{}, invalidArtifact("missing Go version")
	}
	if err := validateDirectory(options.Directory); err != nil {
		return Manifest{}, err
	}
	if err := validateEntries(options.Directory, []string{BinaryName}); err != nil {
		return Manifest{}, err
	}

	binaryPath := filepath.Join(options.Directory, BinaryName)
	digest, err := digestFile(binaryPath)
	if err != nil {
		return Manifest{}, fmt.Errorf("package artifact: %w", err)
	}
	manifest := Manifest{
		BinaryName:   BinaryName,
		BinarySHA256: digest,
		SourceSHA:    options.SourceSHA,
		BuildKey:     options.BuildKey,
		GoVersion:    options.GoVersion,
	}
	manifestData, err := json.Marshal(manifest)
	if err != nil {
		return Manifest{}, fmt.Errorf("marshal artifact manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(options.Directory, ManifestName), manifestData, 0o600); err != nil {
		return Manifest{}, fmt.Errorf("write artifact manifest: %w", err)
	}
	checksum := []byte(digest + "  " + BinaryName + "\n")
	if err := os.WriteFile(filepath.Join(options.Directory, ChecksumName), checksum, 0o600); err != nil {
		return Manifest{}, fmt.Errorf("write artifact checksum: %w", err)
	}
	return manifest, nil
}

// VerifyArtifact checks the exact path, identity, manifest, and checksum.
func VerifyArtifact(options VerifyOptions) (VerifiedArtifact, error) {
	if err := validateArtifactIdentity(options.SourceSHA, options.BuildKey); err != nil {
		return VerifiedArtifact{}, err
	}
	if err := validateDirectory(options.Directory); err != nil {
		return VerifiedArtifact{}, err
	}
	if err := validateEntries(options.Directory, []string{BinaryName, ChecksumName, ManifestName}); err != nil {
		return VerifiedArtifact{}, err
	}

	manifest, err := readManifest(options.Directory, options)
	if err != nil {
		return VerifiedArtifact{}, err
	}

	binaryPath := filepath.Join(options.Directory, BinaryName)
	digest, err := digestFile(binaryPath)
	if err != nil {
		return VerifiedArtifact{}, fmt.Errorf("verify artifact binary: %w", err)
	}
	if digest != manifest.BinarySHA256 {
		return VerifiedArtifact{}, invalidArtifact("binary checksum")
	}
	if err := verifyChecksum(options.Directory, digest); err != nil {
		return VerifiedArtifact{}, err
	}
	return VerifiedArtifact{Manifest: manifest, BinaryPath: binaryPath}, nil
}

func readManifest(directory string, options VerifyOptions) (Manifest, error) {
	manifestData, err := os.ReadFile(filepath.Join(directory, ManifestName))
	if err != nil {
		return Manifest{}, fmt.Errorf("read artifact manifest: %w", err)
	}
	var manifest Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return Manifest{}, invalidArtifact("decode manifest")
	}
	canonicalManifest, err := json.Marshal(manifest)
	if err != nil || !bytes.Equal(canonicalManifest, manifestData) {
		return Manifest{}, invalidArtifact("noncanonical manifest")
	}
	if manifest.BinaryName != BinaryName || manifest.SourceSHA != options.SourceSHA || manifest.BuildKey != options.BuildKey || manifest.GoVersion == "" || !isLowerHex(manifest.BinarySHA256, 64) {
		return Manifest{}, invalidArtifact("manifest identity")
	}
	return manifest, nil
}

func verifyChecksum(directory, digest string) error {
	checksumData, err := os.ReadFile(filepath.Join(directory, ChecksumName))
	if err != nil {
		return fmt.Errorf("read artifact checksum: %w", err)
	}
	if !bytes.Equal(checksumData, []byte(digest+"  "+BinaryName+"\n")) {
		return invalidArtifact("checksum record")
	}
	return nil
}

// ActivateArtifact verifies an artifact before granting executable permission.
func ActivateArtifact(options VerifyOptions) (VerifiedArtifact, error) {
	verified, err := VerifyArtifact(options)
	if err != nil {
		return VerifiedArtifact{}, err
	}
	info, err := os.Lstat(verified.BinaryPath)
	if err != nil || !info.Mode().IsRegular() {
		return VerifiedArtifact{}, invalidArtifact("binary changed after verification")
	}
	if err := os.Chmod(verified.BinaryPath, info.Mode().Perm()|0o111); err != nil {
		return VerifiedArtifact{}, fmt.Errorf("activate artifact: %w", err)
	}
	return verified, nil
}

func validateArtifactIdentity(source, key string) error {
	if !isLowerHex(source, 40) || !isLowerHex(key, 64) {
		return invalidArtifact("malformed identity")
	}
	return nil
}

func validateDirectory(directory string) error {
	if directory == "" {
		return invalidArtifact("missing directory")
	}
	info, err := os.Lstat(directory)
	if err != nil {
		return fmt.Errorf("inspect artifact directory: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return invalidArtifact("artifact directory is not a real directory")
	}
	return nil
}

func validateEntries(directory string, expected []string) error {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return fmt.Errorf("read artifact directory: %w", err)
	}
	if len(entries) != len(expected) {
		return invalidArtifact("unexpected artifact file set")
	}
	wanted := make(map[string]struct{}, len(expected))
	for _, name := range expected {
		wanted[name] = struct{}{}
	}
	for _, entry := range entries {
		if _, ok := wanted[entry.Name()]; !ok {
			return invalidArtifact("unexpected artifact file set")
		}
		info, err := os.Lstat(filepath.Join(directory, entry.Name()))
		if err != nil || !info.Mode().IsRegular() {
			return invalidArtifact("artifact entry is not a regular file")
		}
	}
	return nil
}

func digestFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func invalidArtifact(reason string) error {
	return fmt.Errorf("%w: %s", ErrInvalidArtifact, reason)
}
