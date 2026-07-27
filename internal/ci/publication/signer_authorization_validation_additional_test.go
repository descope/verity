package publication

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errSentinelRunnerFailure = errors.New("sentinel runner failure")

func TestValidate_rejects_missing_boundaries_and_invalid_publication_SHA(t *testing.T) {
	manifest := testManifest(ModeBootstrap, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	options := exactOptions(&manifest)
	tests := []struct {
		name     string
		manifest *Manifest
		options  *ValidationOptions
		wantErr  error
	}{
		{name: "nil manifest", options: &options, wantErr: ErrInvalidManifest},
		{name: "nil options", manifest: &manifest, wantErr: ErrInvalidManifest},
		{name: "invalid manifest shape", manifest: &Manifest{}, options: &options, wantErr: ErrInvalidManifest},
		{
			name: "invalid publication SHA", manifest: &manifest,
			options: func() *ValidationOptions { value := options; value.PublicationSHA = "bad"; return &value }(),
			wantErr: ErrIdentityMismatch,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// When
			err := Validate(context.Background(), test.manifest, test.options)

			// Then
			require.ErrorIs(t, err, test.wantErr)
		})
	}
}

func TestSignerAuthorizationHelpers_reject_invalid_mode_previous_state_and_ancestry(t *testing.T) {
	t.Run("unsupported mode", func(t *testing.T) {
		err := validateModeAuthorization(&Manifest{Mode: "sentinel"}, &ValidationOptions{})
		require.ErrorIs(t, err, ErrInvalidManifest)
	})

	t.Run("invalid candidate manifest", func(t *testing.T) {
		err := validatePreviousManifest(&Manifest{}, nil)
		require.ErrorIs(t, err, ErrInvalidManifest)
	})

	t.Run("delta without previous state", func(t *testing.T) {
		candidate := testManifest(ModeDelta, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
		candidate.PreviousManifestDigest = Digest("sha256:" + strings.Repeat("a", 64))
		err := validatePreviousManifest(&candidate, nil)
		require.ErrorIs(t, err, ErrCASMismatch)
	})

	t.Run("bootstrap over existing state", func(t *testing.T) {
		candidate := testManifest(ModeBootstrap, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
		previous := testManifest(ModeBootstrap, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
		err := validatePreviousManifest(&candidate, &previous)
		require.ErrorIs(t, err, ErrBootstrapUnauthorized)
	})

	t.Run("invalid previous manifest", func(t *testing.T) {
		candidate := testManifest(ModeDelta, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
		previous := Manifest{}
		err := validatePreviousManifest(&candidate, &previous)
		require.ErrorIs(t, err, ErrInvalidManifest)
	})

	t.Run("runner error", func(t *testing.T) {
		err := validateAncestry(context.Background(), &fakeRunner{err: errSentinelRunnerFailure}, ancestryRequest{ancestor: "a", descendant: "b"})
		require.ErrorIs(t, err, errSentinelRunnerFailure)
	})

	t.Run("unexpected exit", func(t *testing.T) {
		err := validateAncestry(context.Background(), &fakeRunner{result: CommandResult{ExitCode: 7, Stderr: []byte("sentinel")}}, ancestryRequest{ancestor: "a", descendant: "b"})
		require.ErrorIs(t, err, ErrAncestryCommandFailed)
	})

	t.Run("component artifact ordering", func(t *testing.T) {
		left := Component{Name: "same", ArtifactName: "a"}
		right := Component{Name: "same", ArtifactName: "b"}
		assert.Negative(t, compareComponent(&left, &right))
	})
}

func TestValidateSigningKeyStateProgression_enforces_epoch_and_revocation_monotonicity(t *testing.T) {
	active := strings.Repeat("a", 64)
	retired := strings.Repeat("b", 64)
	state := func(epoch uint64, activeFingerprint string, trusted, revoked []string) Manifest {
		return Manifest{
			SigningKeyEpoch: epoch, ActiveSigningKeyFingerprint: activeFingerprint,
			TrustedSigningKeyFingerprints: trusted, RevokedSigningKeyFingerprints: revoked,
		}
	}
	tests := []struct {
		name      string
		candidate Manifest
		previous  Manifest
		wantErr   error
	}{
		{name: "missing candidate at epoch zero", candidate: Manifest{}, previous: state(0, active, []string{active}, nil), wantErr: ErrSigningKeyStateChange},
		{name: "missing candidate rolls back epoch", candidate: Manifest{}, previous: state(2, active, []string{active}, nil), wantErr: ErrSigningKeyEpochRollback},
		{name: "first key state", candidate: state(1, active, []string{active}, nil), previous: Manifest{}},
		{name: "epoch rollback", candidate: state(1, active, []string{active}, nil), previous: state(2, active, []string{active}, nil), wantErr: ErrSigningKeyEpochRollback},
		{name: "same epoch unchanged", candidate: state(2, active, []string{active}, nil), previous: state(2, active, []string{active}, nil)},
		{name: "same epoch changed", candidate: state(2, retired, []string{retired}, nil), previous: state(2, active, []string{active}, nil), wantErr: ErrSigningKeyStateChange},
		{name: "revoked becomes active", candidate: state(3, retired, []string{retired}, []string{retired}), previous: state(2, active, []string{active}, []string{retired}), wantErr: ErrSigningKeyRevocationRollback},
		{name: "revoked becomes trusted", candidate: state(3, active, []string{active, retired}, []string{retired}), previous: state(2, active, []string{active}, []string{retired}), wantErr: ErrSigningKeyRevocationRollback},
		{name: "revoked omitted", candidate: state(3, active, []string{active}, nil), previous: state(2, active, []string{active}, []string{retired}), wantErr: ErrSigningKeyRevocationRollback},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// When
			err := validateSigningKeyStateProgression(&test.candidate, &test.previous)

			// Then
			if test.wantErr == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, test.wantErr)
		})
	}
}

func TestValidateRunProgression_rejects_source_change_within_run(t *testing.T) {
	// Given
	previous := testManifest(ModeBootstrap, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	candidate := previous
	candidate.SourceSHA = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	candidate.RunAttempt++

	// When
	err := validateRunProgression(&candidate, &previous)

	// Then
	require.ErrorIs(t, err, ErrIdentityMismatch)
}
