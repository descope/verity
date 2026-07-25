package publication

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMarshalCanonical_is_stable_for_equivalent_ordering(t *testing.T) {
	// Given equivalent manifests with reversed component and operation order.
	first := testManifest(ModeBootstrap, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	first.Components = append(first.Components, Component{
		Name: "charts", Kind: ComponentKindGeneric, ArtifactName: "charts-publication-42-3",
		ArtifactDigest: "sha256:4444444444444444444444444444444444444444444444444444444444444444",
		Workflow:       ".github/workflows/chart-gen.yaml", Event: EventWorkflowCall, Result: ResultSuccess,
	})
	first.APKOperations = append(first.APKOperations, APKOperation{
		Action: APKRemove, Architecture: ArchitectureAArch64, PackageName: "old-package",
	})
	second := first
	second.Components = []Component{first.Components[1], first.Components[0]}
	second.APKOperations = []APKOperation{first.APKOperations[1], first.APKOperations[0]}

	// When both are canonically encoded and digested.
	firstJSON, err := MarshalCanonical(&first)
	require.NoError(t, err)
	secondJSON, err := MarshalCanonical(&second)
	require.NoError(t, err)
	firstDigest, err := DigestManifest(&first)
	require.NoError(t, err)
	secondDigest, err := DigestManifest(&second)
	require.NoError(t, err)

	// Then order does not alter bytes or SHA-256 identity.
	require.Equal(t, firstJSON, secondJSON)
	require.Equal(t, firstDigest, secondDigest)
	sum := sha256.Sum256(firstJSON)
	require.Equal(t, Digest("sha256:"+hex.EncodeToString(sum[:])), firstDigest)
	require.NotContains(t, string(firstJSON), "\n")
}

func TestParseCanonical_accepts_only_exact_typed_encoding(t *testing.T) {
	// Given one canonical manifest artifact.
	manifest := testManifest(ModeBootstrap, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	canonical, err := MarshalCanonical(&manifest)
	require.NoError(t, err)

	// When the exact artifact is parsed.
	parsed, err := ParseCanonical(canonical)

	// Then the typed manifest is recovered.
	require.NoError(t, err)
	require.Equal(t, manifest, parsed)

	for name, input := range map[string][]byte{
		"malformed":      []byte(`{"schema_version":`),
		"unknown field":  append(append([]byte(nil), canonical[:len(canonical)-1]...), []byte(`,"extra":true}`)...),
		"trailing value": append(append([]byte(nil), canonical...), []byte(`{}`)...),
		"whitespace":     append(append([]byte(nil), canonical...), '\n'),
	} {
		t.Run(name, func(t *testing.T) {
			// When malformed or noncanonical bytes are parsed, then they fail closed.
			_, err := ParseCanonical(input)
			require.Error(t, err)
		})
	}
}

func TestMarshalCanonical_rejects_incomplete_duplicate_or_invalid_content(t *testing.T) {
	// Given structurally invalid publication payloads.
	tests := []struct {
		name   string
		mutate func(*Manifest)
	}{
		{"duplicate component", func(m *Manifest) { m.Components = append(m.Components, m.Components[0]) }},
		{"duplicate artifact", func(m *Manifest) {
			duplicate := m.Components[0]
			duplicate.Name = "other"
			m.Components = append(m.Components, duplicate)
		}},
		{"missing workflow", func(m *Manifest) { m.Components[0].Workflow = "" }},
		{"failed result", func(m *Manifest) { m.Components[0].Result = "failure" }},
		{"duplicate operation", func(m *Manifest) { m.APKOperations = append(m.APKOperations, m.APKOperations[0]) }},
		{"upsert without digest", func(m *Manifest) { m.APKOperations[0].ArtifactDigest = "" }},
		{"remove with artifact", func(m *Manifest) { m.APKOperations[0].Action = APKRemove }},
		{"unsupported architecture", func(m *Manifest) { m.APKOperations[0].Architecture = "armv7" }},
		{"unsafe package name", func(m *Manifest) { m.APKOperations[0].PackageName = "../demo" }},
		{"delta without base", func(m *Manifest) { m.Mode = ModeDelta }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifest := testManifest(ModeBootstrap, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
			tt.mutate(&manifest)

			// When canonicalization validates the boundary.
			_, err := MarshalCanonical(&manifest)

			// Then invalid content is rejected before hashing.
			require.ErrorIs(t, err, ErrInvalidManifest)
		})
	}
}

func TestParseCanonical_rejects_one_byte_digest_mutation(t *testing.T) {
	// Given a canonical manifest whose signer digest is changed by one byte.
	manifest := testManifest(ModeBootstrap, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	canonical, err := MarshalCanonical(&manifest)
	require.NoError(t, err)
	mutated := []byte(strings.Replace(string(canonical), strings.Repeat("2", 64), strings.Repeat("2", 63)+"g", 1))

	// When the mutated artifact is parsed.
	_, err = ParseCanonical(mutated)

	// Then the malformed digest is rejected.
	require.ErrorIs(t, err, ErrInvalidManifest)
}
