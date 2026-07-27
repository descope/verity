package publication

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	currentKeyFingerprint = "416d7b8491fccfde1e5d247b4dfc0571ccd20e0610b192334d4ee1308d9adee7"
	retiredKeyFingerprint = "90f7940b20391f49b417b9b3be49f01ee88b975313860b6e1a77bbf7b109c6d2"
)

func TestLoadSigningKeyState_acceptsMatchingCanonicalPublicKey(t *testing.T) {
	// Given a canonical state file and its repository public key.
	repository, statePath := writeSigningKeyStateFixture(t, currentKeyFingerprint)

	// When the trusted state boundary is loaded.
	state, err := LoadSigningKeyState(statePath, repository)

	// Then the verified epoch and trust sets are returned.
	require.NoError(t, err)
	require.Equal(t, uint64(1), state.Epoch)
	require.Equal(t, currentKeyFingerprint, state.ActiveFingerprint)
	require.Equal(t, []string{currentKeyFingerprint}, state.TrustedFingerprints)
	require.Equal(t, []string{retiredKeyFingerprint}, state.RevokedFingerprints)
}

func TestLoadSigningKeyState_rejectsFingerprintMismatch(t *testing.T) {
	// Given state metadata that does not match the committed public key.
	repository, statePath := writeSigningKeyStateFixture(t, retiredKeyFingerprint)

	// When the state is loaded.
	_, err := LoadSigningKeyState(statePath, repository)

	// Then the trust root mismatch fails closed.
	require.ErrorIs(t, err, ErrSigningKeyStateFile)
}

func TestLoadSigningKeyState_rejectsPublicKeyPathEscape(t *testing.T) {
	// Given state metadata that points outside the repository.
	repository := t.TempDir()
	statePath := filepath.Join(t.TempDir(), "state.json")
	data := []byte(`{"schema_version":1,"epoch":1,"public_key_path":"../verity.rsa.pub","active_fingerprint":"` + currentKeyFingerprint + `","trusted_fingerprints":["` + currentKeyFingerprint + `"],"revoked_fingerprints":[]}`)
	require.NoError(t, os.WriteFile(statePath, data, 0o600))

	// When the state is loaded.
	_, err := LoadSigningKeyState(statePath, repository)

	// Then repository traversal is rejected.
	require.ErrorIs(t, err, ErrSigningKeyStateFile)
}

func writeSigningKeyStateFixture(t *testing.T, activeFingerprint string) (repository, statePath string) {
	t.Helper()
	repository = t.TempDir()
	publicKeyPath := filepath.Join(repository, "keys", "apk", "verity.rsa.pub")
	require.NoError(t, os.MkdirAll(filepath.Dir(publicKeyPath), 0o755))
	publicKey, err := os.ReadFile(filepath.Join("..", "..", "..", "keys", "apk", "verity.rsa.pub"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(publicKeyPath, publicKey, 0o644))
	statePath = filepath.Join(t.TempDir(), "state.json")
	data := []byte(`{"schema_version":1,"epoch":1,"public_key_path":"keys/apk/verity.rsa.pub","active_fingerprint":"` + activeFingerprint + `","trusted_fingerprints":["` + activeFingerprint + `"],"revoked_fingerprints":["` + retiredKeyFingerprint + `"]}`)
	require.NoError(t, os.WriteFile(statePath, data, 0o600))
	return repository, statePath
}
