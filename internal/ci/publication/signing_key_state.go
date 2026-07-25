package publication

import (
	"bytes"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const signingKeyStateSchemaVersion = 1

type SigningKeyState struct {
	SchemaVersion       int      `json:"schema_version"`
	Epoch               uint64   `json:"epoch"`
	PublicKeyPath       string   `json:"public_key_path"`
	ActiveFingerprint   string   `json:"active_fingerprint"`
	TrustedFingerprints []string `json:"trusted_fingerprints"`
	RevokedFingerprints []string `json:"revoked_fingerprints"`
}

func LoadSigningKeyState(statePath, repositoryDir string) (SigningKeyState, error) {
	data, err := os.ReadFile(statePath)
	if err != nil {
		return SigningKeyState{}, signingKeyStateError("read %q: %v", statePath, err)
	}
	var state SigningKeyState
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&state); err != nil {
		return SigningKeyState{}, signingKeyStateError("parse %q: %v", statePath, err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return SigningKeyState{}, signingKeyStateError("parse %q: %v", statePath, err)
	}
	if state.SchemaVersion != signingKeyStateSchemaVersion {
		return SigningKeyState{}, signingKeyStateError("schema version %d", state.SchemaVersion)
	}
	if state.PublicKeyPath == "" {
		return SigningKeyState{}, signingKeyStateError("public key path is required")
	}
	publicKey, err := readRepositoryPublicKey(repositoryDir, state.PublicKeyPath)
	if err != nil {
		return SigningKeyState{}, err
	}
	fingerprint, err := canonicalRSAFingerprint(publicKey)
	if err != nil {
		return SigningKeyState{}, err
	}
	if fingerprint != state.ActiveFingerprint {
		return SigningKeyState{}, signingKeyStateError("active fingerprint does not match %q", state.PublicKeyPath)
	}
	manifest := Manifest{
		SigningKeyEpoch: state.Epoch, ActiveSigningKeyFingerprint: state.ActiveFingerprint,
		TrustedSigningKeyFingerprints: state.TrustedFingerprints,
		RevokedSigningKeyFingerprints: state.RevokedFingerprints,
	}
	if err := validateSigningKeyState(&manifest); err != nil {
		return SigningKeyState{}, fmt.Errorf("%w: %w", ErrSigningKeyStateFile, err)
	}
	return state, nil
}

func requireJSONEOF(decoder *json.Decoder) error {
	var trailing json.RawMessage
	err := decoder.Decode(&trailing)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil {
		return err
	}
	return errTrailingJSONValue
}

func readRepositoryPublicKey(repositoryDir, relativePath string) ([]byte, error) {
	if filepath.IsAbs(relativePath) || relativePath != filepath.Clean(relativePath) {
		return nil, signingKeyStateError("public key path %q must be canonical and relative", relativePath)
	}
	repository, err := filepath.EvalSymlinks(repositoryDir)
	if err != nil {
		return nil, signingKeyStateError("resolve repository %q: %v", repositoryDir, err)
	}
	keyPath, err := filepath.EvalSymlinks(filepath.Join(repository, relativePath))
	if err != nil {
		return nil, signingKeyStateError("resolve public key %q: %v", relativePath, err)
	}
	relative, err := filepath.Rel(repository, keyPath)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return nil, signingKeyStateError("public key path %q escapes repository", relativePath)
	}
	info, err := os.Stat(keyPath)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 16<<10 {
		return nil, signingKeyStateError("public key %q must be a regular file of at most 16 KiB", relativePath)
	}
	data, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, signingKeyStateError("read public key %q: %v", relativePath, err)
	}
	return data, nil
}

func canonicalRSAFingerprint(data []byte) (string, error) {
	block, rest := pem.Decode(data)
	if block == nil || block.Type != "PUBLIC KEY" || len(rest) != 0 || len(block.Headers) != 0 {
		return "", signingKeyStateError("public key must contain one canonical SPKI PEM block")
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return "", signingKeyStateError("parse SPKI public key: %v", err)
	}
	publicKey, ok := parsed.(*rsa.PublicKey)
	if !ok || publicKey.N.BitLen() != 4096 || publicKey.E != 65537 {
		return "", signingKeyStateError("public key must be RSA-4096 with exponent 65537")
	}
	canonicalDER, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		return "", signingKeyStateError("marshal SPKI public key: %v", err)
	}
	canonicalPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: canonicalDER})
	if !bytes.Equal(data, canonicalPEM) {
		return "", signingKeyStateError("public key PEM is not canonical")
	}
	digest := sha256.Sum256(canonicalDER)
	return hex.EncodeToString(digest[:]), nil
}

func signingKeyStateError(format string, arguments ...any) error {
	return fmt.Errorf("%w: %s", ErrSigningKeyStateFile, fmt.Sprintf(format, arguments...))
}
