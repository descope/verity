package apkrepository

import (
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
)

func prepareSigningKey(privatePEM []byte, publicKeyPath, destination string) error {
	privateKey, err := parseRSAPrivateKey(privatePEM)
	if err != nil {
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
	if privateKey.E != publicKey.E || privateKey.N.Cmp(publicKey.N) != 0 {
		return fmt.Errorf("%w: %s", errPrivateKeyMismatch, publicKeyPath)
	}
	encoded := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})
	if err := os.WriteFile(destination, encoded, 0o600); err != nil {
		return fmt.Errorf("write signing key: %w", err)
	}
	return nil
}

func parseRSAPrivateKey(data []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errInvalidPrivateKey
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
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
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errInvalidPublicKey
	}
	if key, err := x509.ParsePKCS1PublicKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidPublicKey, err)
	}
	key, ok := parsed.(*rsa.PublicKey)
	if !ok {
		return nil, errInvalidPublicKey
	}
	return key, nil
}
