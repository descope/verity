package sitepublication

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSignerKeyParsers_require_one_canonical_PKCS8_and_SPKI_block(t *testing.T) {
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
	parsedPrivate, privateErr := parseSignerPrivateKey(privatePEM)
	parsedPublic, publicErr := parseSignerPublicKey(publicPEM)

	// Then they round-trip, while alternate formats and trailing data fail closed.
	require.NoError(t, privateErr)
	require.NoError(t, publicErr)
	assert.Equal(t, key.N, parsedPrivate.N)
	assert.Equal(t, key.N, parsedPublic.N)
	privateCases := [][]byte{
		pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}),
		append([]byte("\n"), privatePEM...),
		append(append([]byte(nil), privatePEM...), '\n'),
		append(append([]byte(nil), privatePEM...), privatePEM...),
	}
	for _, data := range privateCases {
		_, parseErr := parseSignerPrivateKey(data)
		require.ErrorIs(t, parseErr, errInvalidSignerPrivateKey)
	}
	publicCases := [][]byte{
		pem.EncodeToMemory(&pem.Block{Type: "RSA PUBLIC KEY", Bytes: x509.MarshalPKCS1PublicKey(&key.PublicKey)}),
		append([]byte("\n"), publicPEM...),
		append(append([]byte(nil), publicPEM...), '\n'),
		append(append([]byte(nil), publicPEM...), publicPEM...),
	}
	for _, data := range publicCases {
		_, parseErr := parseSignerPublicKey(data)
		require.ErrorIs(t, parseErr, errInvalidSignerPublicKey)
	}
}

func TestValidateSignerKeyMaterial_requires_exact_profile_and_matching_key(t *testing.T) {
	// Given generated sentinel material with either a weak profile or mismatched public modulus.
	weakKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	weakDER, err := x509.MarshalPKCS8PrivateKey(weakKey)
	require.NoError(t, err)
	weakPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: weakDER})
	root := t.TempDir()
	plan := SignerPlan{Execution: SignerExecutionSpec{WorkspaceDir: root, PublicKeyPath: "verity.rsa.pub"}}

	// When weak private material is validated.
	weakErr := validateSignerKeyMaterial(&plan, weakPEM)

	// Then the exact RSA-4096/e65537 profile is required.
	require.ErrorIs(t, weakErr, errSignerRSAProfile)

	// Given a valid-profile private key and a different valid-profile public modulus.
	key, err := rsa.GenerateKey(rand.Reader, 4096)
	require.NoError(t, err)
	privateDER, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	privatePEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER})
	otherPublic := rsa.PublicKey{N: new(big.Int).Sub(key.N, big.NewInt(2)), E: 65537}
	publicDER, err := x509.MarshalPKIXPublicKey(&otherPublic)
	require.NoError(t, err)
	publicPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER})
	require.NoError(t, os.WriteFile(filepath.Join(root, plan.Execution.PublicKeyPath), publicPEM, 0o644))

	// When the pair is validated.
	mismatchErr := validateSignerKeyMaterial(&plan, privatePEM)

	// Then matching modulus and exponent are mandatory.
	require.ErrorIs(t, mismatchErr, errSignerKeyMismatch)
}
