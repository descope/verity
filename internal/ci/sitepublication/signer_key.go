package sitepublication

import (
	"bytes"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

var (
	errInvalidSignerPrivateKey = errors.New("invalid signer RSA private key")
	errInvalidSignerPublicKey  = errors.New("invalid signer RSA public key")
	errSignerRSAProfile        = errors.New("signer RSA key must be exactly 4096 bits with exponent 65537")
	errSignerKeyMismatch       = errors.New("signer private key does not match published public key")
)

func validateSignerKeyMaterial(plan *SignerPlan, privatePEM []byte) error {
	privateKey, err := parseSignerPrivateKey(privatePEM)
	if err != nil {
		return err
	}
	if err := validateSignerRSAProfile(&privateKey.PublicKey); err != nil {
		return err
	}
	if err := privateKey.Validate(); err != nil {
		return errInvalidSignerPrivateKey
	}
	publicPath := filepath.Join(plan.Execution.WorkspaceDir, filepath.FromSlash(plan.Execution.PublicKeyPath))
	publicPEM, err := os.ReadFile(publicPath)
	if err != nil {
		return fmt.Errorf("read signer public key: %w", err)
	}
	publicKey, err := parseSignerPublicKey(publicPEM)
	if err != nil {
		return err
	}
	if err := validateSignerRSAProfile(publicKey); err != nil {
		return err
	}
	if privateKey.E != publicKey.E || privateKey.N.Cmp(publicKey.N) != 0 {
		return errSignerKeyMismatch
	}
	return nil
}

func parseSignerPrivateKey(data []byte) (*rsa.PrivateKey, error) {
	der, err := decodeCanonicalSignerPEM(data, "PRIVATE KEY", errInvalidSignerPrivateKey)
	if err != nil {
		return nil, err
	}
	defer clear(der)
	parsed, err := x509.ParsePKCS8PrivateKey(der)
	if err != nil {
		return nil, errInvalidSignerPrivateKey
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, errInvalidSignerPrivateKey
	}
	return key, nil
}

func parseSignerPublicKey(data []byte) (*rsa.PublicKey, error) {
	der, err := decodeCanonicalSignerPEM(data, "PUBLIC KEY", errInvalidSignerPublicKey)
	if err != nil {
		return nil, err
	}
	parsed, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		return nil, errInvalidSignerPublicKey
	}
	key, ok := parsed.(*rsa.PublicKey)
	if !ok {
		return nil, errInvalidSignerPublicKey
	}
	return key, nil
}

func validateSignerRSAProfile(key *rsa.PublicKey) error {
	if key == nil || key.N == nil || key.N.BitLen() != 4096 || key.E != 65537 {
		return errSignerRSAProfile
	}
	return nil
}

func decodeCanonicalSignerPEM(data []byte, blockType string, invalidError error) ([]byte, error) {
	block, rest := pem.Decode(data)
	if block == nil || len(rest) != 0 || block.Type != blockType || len(block.Headers) != 0 {
		return nil, invalidError
	}
	canonical := pem.EncodeToMemory(block)
	defer clear(canonical)
	if !bytes.Equal(data, canonical) {
		return nil, invalidError
	}
	return block.Bytes, nil
}
