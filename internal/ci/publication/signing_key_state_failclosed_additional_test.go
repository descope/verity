package publication

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadSigningKeyState_rejects_missing_malformed_and_incomplete_state_files(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{name: "malformed", data: `{`},
		{name: "unknown field", data: `{"unknown":true}`},
		{name: "trailing value", data: `{"schema_version":1}{}`},
		{name: "malformed trailing value", data: `{"schema_version":1}x`},
		{name: "wrong schema", data: `{"schema_version":2}`},
		{name: "missing public key path", data: `{"schema_version":1}`},
	}

	t.Run("missing file", func(t *testing.T) {
		// When
		state, err := LoadSigningKeyState(filepath.Join(t.TempDir(), "missing.json"), t.TempDir())

		// Then
		require.ErrorIs(t, err, ErrSigningKeyStateFile)
		assert.Equal(t, SigningKeyState{}, state)
	})

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given
			statePath := filepath.Join(t.TempDir(), "state.json")
			require.NoError(t, os.WriteFile(statePath, []byte(test.data), 0o600))

			// When
			state, err := LoadSigningKeyState(statePath, t.TempDir())

			// Then
			require.ErrorIs(t, err, ErrSigningKeyStateFile)
			assert.Equal(t, SigningKeyState{}, state)
		})
	}
}

func TestReadRepositoryPublicKey_rejects_escape_missing_and_nonregular_paths(t *testing.T) {
	t.Run("absolute path", func(t *testing.T) {
		_, err := readRepositoryPublicKey(t.TempDir(), filepath.Join(string(filepath.Separator), "sentinel.pub"))
		require.ErrorIs(t, err, ErrSigningKeyStateFile)
	})

	t.Run("noncanonical path", func(t *testing.T) {
		_, err := readRepositoryPublicKey(t.TempDir(), "keys/../sentinel.pub")
		require.ErrorIs(t, err, ErrSigningKeyStateFile)
	})

	t.Run("missing repository", func(t *testing.T) {
		_, err := readRepositoryPublicKey(filepath.Join(t.TempDir(), "missing"), "sentinel.pub")
		require.ErrorIs(t, err, ErrSigningKeyStateFile)
	})

	t.Run("missing key", func(t *testing.T) {
		_, err := readRepositoryPublicKey(t.TempDir(), "sentinel.pub")
		require.ErrorIs(t, err, ErrSigningKeyStateFile)
	})

	t.Run("symlink escape", func(t *testing.T) {
		// Given
		repository := t.TempDir()
		outside := filepath.Join(t.TempDir(), "outside.pub")
		require.NoError(t, os.WriteFile(outside, []byte("sentinel"), 0o600))
		require.NoError(t, os.Symlink(outside, filepath.Join(repository, "sentinel.pub")))

		// When
		_, err := readRepositoryPublicKey(repository, "sentinel.pub")

		// Then
		require.ErrorIs(t, err, ErrSigningKeyStateFile)
	})

	t.Run("directory", func(t *testing.T) {
		repository := t.TempDir()
		require.NoError(t, os.Mkdir(filepath.Join(repository, "sentinel.pub"), 0o755))
		_, err := readRepositoryPublicKey(repository, "sentinel.pub")
		require.ErrorIs(t, err, ErrSigningKeyStateFile)
	})

	t.Run("oversized", func(t *testing.T) {
		repository := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(repository, "sentinel.pub"), make([]byte, (16<<10)+1), 0o600))
		_, err := readRepositoryPublicKey(repository, "sentinel.pub")
		require.ErrorIs(t, err, ErrSigningKeyStateFile)
	})

	t.Run("regular sentinel", func(t *testing.T) {
		repository := t.TempDir()
		path := filepath.Join(repository, "sentinel.pub")
		require.NoError(t, os.WriteFile(path, []byte("sentinel-public-key"), 0o600))
		data, err := readRepositoryPublicKey(repository, "sentinel.pub")
		require.NoError(t, err)
		assert.Equal(t, []byte("sentinel-public-key"), data)
	})
}

func TestCanonicalRSAFingerprint_rejects_noncanonical_or_wrong_profile_keys(t *testing.T) {
	// Given
	ecdsaKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	ecdsaDER, err := x509.MarshalPKIXPublicKey(&ecdsaKey.PublicKey)
	require.NoError(t, err)
	weakRSA, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	weakDER, err := x509.MarshalPKIXPublicKey(&weakRSA.PublicKey)
	require.NoError(t, err)
	tests := []struct {
		name string
		data []byte
	}{
		{name: "not PEM", data: []byte("sentinel")},
		{name: "wrong block type", data: pem.EncodeToMemory(&pem.Block{Type: "RSA PUBLIC KEY", Bytes: weakDER})},
		{name: "PEM headers", data: pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: weakDER, Headers: map[string]string{"Sentinel": "true"}})},
		{name: "invalid DER", data: pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: []byte("sentinel")})},
		{name: "non RSA", data: pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: ecdsaDER})},
		{name: "weak RSA", data: pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: weakDER})},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// When
			fingerprint, err := canonicalRSAFingerprint(test.data)

			// Then
			require.ErrorIs(t, err, ErrSigningKeyStateFile)
			assert.Empty(t, fingerprint)
		})
	}

	// Given a valid-profile key encoded with noncanonical PEM line wrapping.
	strongRSA, err := rsa.GenerateKey(rand.Reader, 4096)
	require.NoError(t, err)
	strongDER, err := x509.MarshalPKIXPublicKey(&strongRSA.PublicKey)
	require.NoError(t, err)
	noncanonical := []byte("-----BEGIN PUBLIC KEY-----\n" + base64.StdEncoding.EncodeToString(strongDER) + "\n-----END PUBLIC KEY-----\n")

	// When
	fingerprint, err := canonicalRSAFingerprint(noncanonical)

	// Then
	require.ErrorIs(t, err, ErrSigningKeyStateFile)
	assert.Empty(t, fingerprint)
	assert.NotContains(t, string(noncanonical), strings.Repeat("\n", 2))
}
