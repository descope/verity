package apkrepository

import (
	"bytes"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
)

var (
	errInvalidPrivateKey = errors.New("invalid RSA private key")
	errInvalidPublicKey  = errors.New("invalid RSA public key")
	errInvalidRSAProfile = errors.New("RSA key must be exactly 4096 bits with exponent 65537")
)

func prepareSigningKey(privatePEM []byte, publicKeyPath, destination string) error {
	privateKey, err := parseRSAPrivateKey(privatePEM)
	if err != nil {
		return err
	}
	if err := validateRSAPrivateKey(privateKey); err != nil {
		return err
	}
	publicPEM, err := os.ReadFile(publicKeyPath)
	if err != nil {
		return fmt.Errorf("APK repository public key not found: %s: %w", publicKeyPath, err)
	}
	publicKey, err := parseRSAPublicKey(publicPEM)
	if err != nil {
		return err
	}
	if err := validateRSAPublicKey(publicKey); err != nil {
		return err
	}
	if privateKey.E != publicKey.E || privateKey.N.Cmp(publicKey.N) != 0 {
		return fmt.Errorf("%w: %s", errPrivateKeyMismatch, publicKeyPath)
	}
	der := x509.MarshalPKCS1PrivateKey(privateKey)
	defer clear(der)
	encoded := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: der})
	defer clear(encoded)
	file, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create signing key: %w", err)
	}
	_, writeErr := file.Write(encoded)
	closeErr := file.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		cleanupErr := os.Remove(destination)
		if errors.Is(cleanupErr, os.ErrNotExist) {
			cleanupErr = nil
		}
		return fmt.Errorf("write signing key: %w", errors.Join(err, cleanupErr))
	}
	return nil
}

func validateRSAPrivateKey(key *rsa.PrivateKey) error {
	if key == nil || key.N == nil || key.N.BitLen() != 4096 || key.E != 65537 {
		return errInvalidRSAProfile
	}
	if err := key.Validate(); err != nil {
		return fmt.Errorf("%w: private key consistency check failed", errInvalidPrivateKey)
	}
	return nil
}

func validateRSAPublicKey(key *rsa.PublicKey) error {
	if key == nil || key.N == nil || key.N.BitLen() != 4096 || key.E != 65537 {
		return errInvalidRSAProfile
	}
	return nil
}

func parseRSAPrivateKey(data []byte) (*rsa.PrivateKey, error) {
	der, err := decodeCanonicalPEM(data, "PRIVATE KEY", errInvalidPrivateKey)
	if err != nil {
		return nil, err
	}
	defer clear(der)
	parsed, err := x509.ParsePKCS8PrivateKey(der)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidPrivateKey, err)
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, errInvalidPrivateKey
	}
	return key, nil
}

func parseRSAPublicKey(data []byte) (*rsa.PublicKey, error) {
	der, err := decodeCanonicalPEM(data, "PUBLIC KEY", errInvalidPublicKey)
	if err != nil {
		return nil, err
	}
	parsed, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidPublicKey, err)
	}
	key, ok := parsed.(*rsa.PublicKey)
	if !ok {
		return nil, errInvalidPublicKey
	}
	return key, nil
}

func decodeCanonicalPEM(data []byte, blockType string, invalidError error) ([]byte, error) {
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
