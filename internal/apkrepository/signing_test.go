package apkrepository

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrepareSigningKey_writes_matching_PKCS1_private_key(t *testing.T) {
	// Given a matching PKCS8 private key and PKIX public key.
	privatePEM, publicPEM := testRSAKeyPair(t)
	root := t.TempDir()
	publicPath := filepath.Join(root, "verity.rsa.pub")
	destination := filepath.Join(root, "verity.rsa")
	require.NoError(t, os.WriteFile(publicPath, publicPEM, 0o644))

	// When the signing key is prepared for abuild-sign.
	err := prepareSigningKey(privatePEM, publicPath, destination)

	// Then the traditional RSA private-key form is written with private permissions.
	require.NoError(t, err)
	contents, readErr := os.ReadFile(destination)
	require.NoError(t, readErr)
	block, _ := pem.Decode(contents)
	require.NotNil(t, block)
	assert.Equal(t, "RSA PRIVATE KEY", block.Type)
	_, parseErr := x509.ParsePKCS1PrivateKey(block.Bytes)
	require.NoError(t, parseErr)
	info, statErr := os.Stat(destination)
	require.NoError(t, statErr)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestPrepareSigningKey_rejects_mismatched_public_key(t *testing.T) {
	// Given a private key and a different public key.
	privatePEM, _ := testRSAKeyPair(t)
	_, otherPublicPEM := testRSAKeyPair(t)
	publicPath := filepath.Join(t.TempDir(), "verity.rsa.pub")
	require.NoError(t, os.WriteFile(publicPath, otherPublicPEM, 0o644))

	// When the signing key is prepared.
	err := prepareSigningKey(privatePEM, publicPath, filepath.Join(t.TempDir(), "verity.rsa"))

	// Then publication fails before the secret can sign packages.
	require.ErrorIs(t, err, errPrivateKeyMismatch)
}

func TestRSAKeyParsers_accept_PKCS1_and_reject_invalid_material(t *testing.T) {
	// Given one generated RSA key in traditional private/public encodings.
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	privatePEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	publicPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PUBLIC KEY", Bytes: x509.MarshalPKCS1PublicKey(&key.PublicKey)})

	// When both parser variants are used.
	parsedPrivate, privateErr := parseRSAPrivateKey(privatePEM)
	parsedPublic, publicErr := parseRSAPublicKey(publicPEM)

	// Then the keys round-trip and malformed PEM fails closed.
	require.NoError(t, privateErr)
	require.NoError(t, publicErr)
	assert.Equal(t, key.N, parsedPrivate.N)
	assert.Equal(t, key.N, parsedPublic.N)
	_, invalidPrivateErr := parseRSAPrivateKey([]byte("invalid"))
	_, invalidPublicErr := parseRSAPublicKey([]byte("invalid"))
	require.ErrorIs(t, invalidPrivateErr, errInvalidPrivateKey)
	require.ErrorIs(t, invalidPublicErr, errInvalidPublicKey)
}

func testRSAKeyPair(t *testing.T) (privatePEM, publicPEM []byte) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	privateDER, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	publicDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	require.NoError(t, err)
	privatePEM = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER})
	publicPEM = pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER})
	return privatePEM, publicPEM
}
