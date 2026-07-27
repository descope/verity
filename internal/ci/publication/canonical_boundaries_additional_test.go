package publication

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMarshalCanonical_rejects_nil_manifest(t *testing.T) {
	// When
	data, err := MarshalCanonical(nil)

	// Then
	require.ErrorIs(t, err, ErrInvalidManifest)
	assert.Nil(t, data)
}

func TestCanonicalManifestValue_preserves_explicit_empty_signing_key_sets(t *testing.T) {
	// Given
	manifest := testManifest(ModeBootstrap, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	manifest.SigningKeyEpoch = 1
	manifest.ActiveSigningKeyFingerprint = strings.Repeat("a", 64)
	manifest.TrustedSigningKeyFingerprints = []string{}
	manifest.RevokedSigningKeyFingerprints = []string{}

	// When
	canonical := canonicalManifestValue(&manifest)

	// Then
	require.NotNil(t, canonical.SigningKeyEpoch)
	require.NotNil(t, canonical.TrustedSigningKeyFingerprints)
	require.NotNil(t, canonical.RevokedSigningKeyFingerprints)
	assert.Empty(t, *canonical.TrustedSigningKeyFingerprints)
	assert.Empty(t, *canonical.RevokedSigningKeyFingerprints)
}

func TestParseCanonical_rejects_trailing_syntax_and_partial_signing_key_state(t *testing.T) {
	// Given
	manifest := testManifest(ModeBootstrap, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	canonical, err := MarshalCanonical(&manifest)
	require.NoError(t, err)
	base := strings.TrimSuffix(string(canonical), "}")
	tests := []struct {
		name string
		data string
	}{
		{name: "invalid trailing syntax", data: string(canonical) + "x"},
		{name: "partial key state", data: base + `,"signing_key_epoch":1}`},
		{
			name: "null key epoch",
			data: base + `,"signing_key_epoch":null,"active_signing_key_fingerprint":"","trusted_signing_key_fingerprints":[],"revoked_signing_key_fingerprints":[]}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// When
			parsed, err := ParseCanonical([]byte(test.data))

			// Then
			require.ErrorIs(t, err, ErrInvalidManifest)
			assert.Equal(t, Manifest{}, parsed)
		})
	}
}

func TestComponentsCanonical_accepts_exact_ordered_output_and_rejects_ambiguous_input(t *testing.T) {
	// Given
	components := testManifest(ModeBootstrap, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa").Components
	canonical, err := MarshalComponentsCanonical(components)
	require.NoError(t, err)

	// When
	parsed, err := ParseComponentsCanonical(canonical)

	// Then
	require.NoError(t, err)
	assert.Equal(t, components, parsed)

	tests := []struct {
		name    string
		data    string
		wantErr error
	}{
		{name: "malformed", data: `[`, wantErr: ErrInvalidManifest},
		{name: "unknown field", data: `[{"unknown":true}]`, wantErr: ErrInvalidManifest},
		{name: "trailing value", data: string(canonical) + `{}`, wantErr: ErrInvalidManifest},
		{name: "invalid components", data: `[]`, wantErr: ErrInvalidManifest},
		{name: "noncanonical whitespace", data: string(canonical) + "\n", wantErr: ErrNonCanonicalManifest},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// When
			value, err := ParseComponentsCanonical([]byte(test.data))

			// Then
			require.ErrorIs(t, err, test.wantErr)
			assert.Nil(t, value)
		})
	}

	// When invalid components are marshaled directly.
	data, err := MarshalComponentsCanonical(nil)

	// Then
	require.ErrorIs(t, err, ErrInvalidManifest)
	assert.Nil(t, data)
}
