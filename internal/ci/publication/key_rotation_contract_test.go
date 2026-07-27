package publication

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	activeKeyFingerprint  = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	overlapKeyFingerprint = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

func TestParseCanonical_accepts_active_key_overlap_and_revocation_state(t *testing.T) {
	// Given a canonical manifest with one active key, one overlap key, and no revocations.
	manifest := testManifest(ModeBootstrap, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	canonical, err := MarshalCanonical(&manifest)
	require.NoError(t, err)
	rotated := injectSigningKeyState(t, canonical, 7, activeKeyFingerprint, []string{activeKeyFingerprint, overlapKeyFingerprint}, nil)

	// When the rotation-aware manifest is parsed.
	_, err = ParseCanonical(rotated)

	// Then the bounded overlap state is accepted as part of the signed contract.
	require.NoError(t, err)
}

func TestParseCanonical_rejects_active_key_that_is_revoked(t *testing.T) {
	// Given a manifest that marks its active key as revoked.
	manifest := testManifest(ModeBootstrap, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	canonical, err := MarshalCanonical(&manifest)
	require.NoError(t, err)
	invalid := injectSigningKeyState(t, canonical, 7, activeKeyFingerprint, []string{activeKeyFingerprint}, []string{activeKeyFingerprint})

	// When the contradictory key state is parsed.
	_, err = ParseCanonical(invalid)

	// Then publication fails closed.
	require.ErrorIs(t, err, ErrInvalidManifest)
	assert.Contains(t, strings.ToLower(err.Error()), "active signing key is revoked")
}

func TestValidate_rejects_signing_key_epoch_rollback(t *testing.T) {
	// Given a previous epoch 7 manifest and a candidate that attempts epoch 6.
	previousBase := testManifest(ModeBootstrap, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	previousCanonical, err := MarshalCanonical(&previousBase)
	require.NoError(t, err)
	previousCanonical = injectSigningKeyState(t, previousCanonical, 7, activeKeyFingerprint, []string{activeKeyFingerprint}, nil)
	previous, err := ParseCanonical(previousCanonical)
	require.NoError(t, err)
	previousSum := sha256.Sum256(previousCanonical)
	candidateBase := testManifest(ModeDelta, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	candidateBase.PreviousManifestDigest = Digest("sha256:" + hex.EncodeToString(previousSum[:]))
	candidateCanonical, err := MarshalCanonical(&candidateBase)
	require.NoError(t, err)
	candidateCanonical = injectSigningKeyState(t, candidateCanonical, 6, activeKeyFingerprint, []string{activeKeyFingerprint}, nil)
	candidate, err := ParseCanonical(candidateCanonical)
	require.NoError(t, err)
	options := exactOptions(&candidate)
	options.PreviousManifest = &previous
	options.Runner = &fakeRunner{result: CommandResult{ExitCode: 0}}

	// When publication validation compares the key epochs.
	err = Validate(context.Background(), &candidate, &options)

	// Then stale trust state cannot be resurrected through rollback.
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "key epoch")
}

func TestValidate_rejects_higher_epoch_revoked_key_resurrection_for_normal_and_restore(t *testing.T) {
	resurrections := []struct {
		name    string
		active  string
		trusted []string
	}{
		{
			name:    "active key",
			active:  overlapKeyFingerprint,
			trusted: []string{overlapKeyFingerprint},
		},
		{
			name:    "trusted overlap",
			active:  activeKeyFingerprint,
			trusted: []string{activeKeyFingerprint, overlapKeyFingerprint},
		},
	}
	for _, mode := range []Mode{ModeDelta, ModeRestore} {
		for _, resurrection := range resurrections {
			t.Run(string(mode)+"/"+resurrection.name, func(t *testing.T) {
				// Given a revoked fingerprint from epoch 7 is resurrected in a valid epoch 8 state.
				previous := testManifest(ModeBootstrap, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
				previous.RunID = 41
				previous.RunAttempt = 2
				previous.BatchID = "41-2"
				previous.SigningKeyEpoch = 7
				previous.ActiveSigningKeyFingerprint = activeKeyFingerprint
				previous.TrustedSigningKeyFingerprints = []string{activeKeyFingerprint}
				previous.RevokedSigningKeyFingerprints = []string{overlapKeyFingerprint}
				previousDigest, err := DigestManifest(&previous)
				require.NoError(t, err)

				candidate := testManifest(mode, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
				candidate.PreviousManifestDigest = previousDigest
				candidate.SigningKeyEpoch = 8
				candidate.ActiveSigningKeyFingerprint = resurrection.active
				candidate.TrustedSigningKeyFingerprints = resurrection.trusted
				options := exactOptions(&candidate)
				options.PreviousManifest = &previous
				options.AuthorizeRestore = mode == ModeRestore
				options.Runner = &fakeRunner{result: CommandResult{ExitCode: 0}}

				// When publication validation compares the higher-epoch key state.
				err = Validate(context.Background(), &candidate, &options)

				// Then neither normal publication nor restore can resurrect revoked trust.
				require.ErrorIs(t, err, ErrSigningKeyRevocationRollback)
				assert.Contains(t, strings.ToLower(err.Error()), "revoked")
			})
		}
	}
}

func injectSigningKeyState(t *testing.T, canonical []byte, epoch uint64, active string, trusted, revoked []string) []byte {
	t.Helper()
	trustedJSON, err := json.Marshal(trusted)
	require.NoError(t, err)
	revokedJSON, err := json.Marshal(revoked)
	require.NoError(t, err)
	state := fmt.Sprintf(
		`,"signing_key_epoch":%d,"active_signing_key_fingerprint":%q,"trusted_signing_key_fingerprints":%s,"revoked_signing_key_fingerprints":%s`,
		epoch, active, trustedJSON, revokedJSON,
	)
	result := strings.Replace(string(canonical), `,"apk_operations":`, state+`,"apk_operations":`, 1)
	require.NotEqual(t, string(canonical), result)
	return []byte(result)
}
