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

	// When the signing key is prepared for Melange.
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

func TestRSAKeyParsers_require_one_canonical_PKCS8_and_SPKI_block(t *testing.T) {
	// Given one generated sentinel RSA key in canonical and forbidden encodings.
	key, err := rsa.GenerateKey(rand.Reader, 4096)
	require.NoError(t, err)
	privateDER, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	publicDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	require.NoError(t, err)
	privatePEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER})
	publicPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER})

	// When the canonical encodings are parsed.
	parsedPrivate, privateErr := parseRSAPrivateKey(privatePEM)
	parsedPublic, publicErr := parseRSAPublicKey(publicPEM)

	// Then they round-trip, while alternate formats and trailing data fail closed.
	require.NoError(t, privateErr)
	require.NoError(t, publicErr)
	assert.Equal(t, key.N, parsedPrivate.N)
	assert.Equal(t, key.N, parsedPublic.N)
	privateCases := [][]byte{
		[]byte("invalid"),
		pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}),
		append([]byte("\n"), privatePEM...),
		append(append([]byte(nil), privatePEM...), '\n'),
		append(append([]byte(nil), privatePEM...), privatePEM...),
	}
	for _, data := range privateCases {
		_, parseErr := parseRSAPrivateKey(data)
		require.ErrorIs(t, parseErr, errInvalidPrivateKey)
	}
	publicCases := [][]byte{
		[]byte("invalid"),
		pem.EncodeToMemory(&pem.Block{Type: "RSA PUBLIC KEY", Bytes: x509.MarshalPKCS1PublicKey(&key.PublicKey)}),
		append([]byte("\n"), publicPEM...),
		append(append([]byte(nil), publicPEM...), '\n'),
		append(append([]byte(nil), publicPEM...), publicPEM...),
	}
	for _, data := range publicCases {
		_, parseErr := parseRSAPublicKey(data)
		require.ErrorIs(t, parseErr, errInvalidPublicKey)
	}
}

func TestRSAKeyValidation_requires_exact_4096_bit_65537_profile(t *testing.T) {
	// Given generated sentinel keys outside the production profile.
	weakExponent, err := rsa.GenerateKey(rand.Reader, 4096)
	require.NoError(t, err)
	weakExponent.E = 3
	weakSize, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	// When private and public profiles are validated.
	privateExponentErr := validateRSAPrivateKey(weakExponent)
	publicExponentErr := validateRSAPublicKey(&weakExponent.PublicKey)
	privateSizeErr := validateRSAPrivateKey(weakSize)
	publicSizeErr := validateRSAPublicKey(&weakSize.PublicKey)

	// Then every non-exact key fails closed.
	require.ErrorIs(t, privateExponentErr, errInvalidRSAProfile)
	require.ErrorIs(t, publicExponentErr, errInvalidRSAProfile)
	require.ErrorIs(t, privateSizeErr, errInvalidRSAProfile)
	require.ErrorIs(t, publicSizeErr, errInvalidRSAProfile)
}

func testRSAKeyPair(t *testing.T) (privatePEM, publicPEM []byte) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 4096)
	require.NoError(t, err)
	privateDER, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	publicDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	require.NoError(t, err)
	privatePEM = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER})
	publicPEM = pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER})
	return privatePEM, publicPEM
}
