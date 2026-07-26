package publication

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseAPKOperationsCanonical_accepts_sorted_exact_operations(t *testing.T) {
	// Given
	operations := []APKOperation{
		{Action: APKRemove, Architecture: ArchitectureAArch64, PackageName: "alpha"},
		{Action: APKRemove, Architecture: ArchitectureX8664, PackageName: "alpha"},
		{Action: APKUpsert, Architecture: ArchitectureX8664, PackageName: "alpha"},
		{Action: APKUpsert, Architecture: ArchitectureX8664, PackageName: "zeta"},
	}
	data, err := json.Marshal(operations)
	require.NoError(t, err)

	// When
	parsed, err := ParseAPKOperationsCanonical(data)

	// Then
	require.NoError(t, err)
	assert.Equal(t, operations, parsed)
}

func TestParseAPKOperationsCanonical_rejects_ambiguous_or_noncanonical_input(t *testing.T) {
	tests := []struct {
		name    string
		data    string
		wantErr error
	}{
		{name: "malformed JSON", data: `[`, wantErr: ErrComposeInvalid},
		{name: "unknown field", data: `[{"action":"remove","architecture":"aarch64","package_name":"alpha","unknown":true}]`, wantErr: ErrComposeInvalid},
		{name: "duplicate field", data: `[{"action":"remove","action":"upsert","architecture":"aarch64","package_name":"alpha"}]`, wantErr: ErrComposeInvalid},
		{name: "trailing JSON", data: `[]{}`, wantErr: ErrComposeInvalid},
		{name: "trailing whitespace", data: "[]\n", wantErr: ErrNonCanonicalManifest},
		{
			name:    "unsorted architecture",
			data:    `[{"action":"upsert","architecture":"x86_64","package_name":"zeta","artifact_name":"","artifact_digest":""},{"action":"remove","architecture":"aarch64","package_name":"alpha","artifact_name":"","artifact_digest":""}]`,
			wantErr: ErrNonCanonicalManifest,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// When
			operations, err := ParseAPKOperationsCanonical([]byte(test.data))

			// Then
			require.ErrorIs(t, err, test.wantErr)
			assert.Nil(t, operations)
		})
	}
}
