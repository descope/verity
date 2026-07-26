package publication

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	ci "github.com/verity-org/verity/internal/ci"
)

func TestParseAPKOperationsCanonical_accepts_only_exact_sorted_bytes(t *testing.T) {
	// Given
	operations := []APKOperation{
		{Action: APKRemove, Architecture: ArchitectureAArch64, PackageName: "alpha"},
		{Action: APKUpsert, Architecture: ArchitectureAArch64, PackageName: "alpha", ArtifactName: "artifact-alpha", ArtifactDigest: digestSeed("1")},
		{Action: APKUpsert, Architecture: ArchitectureAArch64, PackageName: "beta", ArtifactName: "artifact-beta", ArtifactDigest: digestSeed("2")},
		{Action: APKUpsert, Architecture: ArchitectureX8664, PackageName: "alpha", ArtifactName: "artifact-alpha", ArtifactDigest: digestSeed("3")},
	}
	data, err := json.Marshal(operations)
	require.NoError(t, err)

	// When
	parsed, err := ParseAPKOperationsCanonical(data)

	// Then
	require.NoError(t, err)
	require.Equal(t, operations, parsed)
}

func TestParseAPKOperationsCanonical_rejects_malformed_ambiguous_trailing_and_unsorted_bytes(t *testing.T) {
	unsorted, err := json.Marshal([]APKOperation{
		{Action: APKUpsert, Architecture: ArchitectureX8664, PackageName: "zeta"},
		{Action: APKUpsert, Architecture: ArchitectureAArch64, PackageName: "zeta"},
		{Action: APKUpsert, Architecture: ArchitectureAArch64, PackageName: "alpha"},
		{Action: APKRemove, Architecture: ArchitectureAArch64, PackageName: "alpha"},
	})
	require.NoError(t, err)
	tests := []struct {
		name    string
		data    []byte
		wantErr error
	}{
		{name: "malformed", data: []byte(`[`), wantErr: ErrComposeInvalid},
		{name: "duplicate key", data: []byte(`[{"action":"remove","action":"upsert","architecture":"x86_64","package_name":"demo"}]`), wantErr: ErrComposeInvalid},
		{name: "unknown field", data: []byte(`[{"action":"remove","architecture":"x86_64","package_name":"demo","unknown":true}]`), wantErr: ErrComposeInvalid},
		{name: "trailing value", data: []byte(`[] {}`), wantErr: ErrComposeInvalid},
		{name: "unsorted", data: unsorted, wantErr: ErrNonCanonicalManifest},
		{name: "insignificant whitespace", data: []byte("[]\n"), wantErr: ErrNonCanonicalManifest},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given
			// The table provides one exact hostile or non-canonical byte sequence.

			// When
			_, err := ParseAPKOperationsCanonical(test.data)

			// Then
			require.ErrorIs(t, err, test.wantErr)
		})
	}
}

func TestCompose_rejects_nil_previous_digest_validation_and_conflicting_operation_inputs(t *testing.T) {
	integerData := composeIntegerManifest(t)
	chartData := composeChartManifest("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	tests := []struct {
		name    string
		request func() *ComposeRequest
		wantErr error
	}{
		{name: "nil request", request: func() *ComposeRequest { return nil }, wantErr: ErrComposeInvalid},
		{name: "digest without previous manifest", request: func() *ComposeRequest {
			request := composeRequest(integerData, chartData)
			request.PreviousManifestDigest = digestSeed("4")
			return &request
		}, wantErr: ErrComposeInvalid},
		{name: "mutually exclusive operation inputs", request: func() *ComposeRequest {
			request := composeRequest(integerData, chartData)
			request.APKOperations = []APKOperation{}
			request.APKDelta = []byte(`{}`)
			return &request
		}, wantErr: ErrComposeInvalid},
		{name: "bootstrap validation", request: func() *ComposeRequest {
			request := composeRequest(integerData, chartData)
			request.AuthorizeBootstrap = false
			return &request
		}, wantErr: ErrBootstrapUnauthorized},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given
			request := test.request()

			// When
			_, err := Compose(context.Background(), request)

			// Then
			require.ErrorIs(t, err, test.wantErr)
		})
	}
}

func TestResolvePreviousDigest_validates_absence_shape_and_exact_digest(t *testing.T) {
	// Given
	previous := testManifest(ModeBootstrap, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	digest, err := DigestManifest(&previous)
	require.NoError(t, err)
	tests := []struct {
		name    string
		request ComposeRequest
		want    Digest
		wantErr error
	}{
		{name: "absent", request: ComposeRequest{}},
		{name: "invalid previous shape", request: ComposeRequest{PreviousManifest: &Manifest{}}, wantErr: ErrInvalidManifest},
		{name: "matching digest", request: ComposeRequest{PreviousManifest: &previous, PreviousManifestDigest: digest}, want: digest},
		{name: "computed digest", request: ComposeRequest{PreviousManifest: &previous}, want: digest},
		{name: "mismatched digest", request: ComposeRequest{PreviousManifest: &previous, PreviousManifestDigest: digestSeed("9")}, wantErr: ErrProducerConflict},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// When
			got, err := resolvePreviousDigest(&test.request)

			// Then
			if test.wantErr != nil {
				require.ErrorIs(t, err, test.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.want, got)
		})
	}
}

func TestComposeAPKOperations_copies_explicit_maps_default_and_materializes_delta(t *testing.T) {
	manifest, err := ci.ParseIntegerBatchManifest(composeIntegerManifest(t))
	require.NoError(t, err)

	t.Run("explicit operations are copied", func(t *testing.T) {
		// Given
		request := ComposeRequest{APKOperations: []APKOperation{{Action: APKRemove, Architecture: ArchitectureX8664, PackageName: "sentinel"}}}

		// When
		operations, err := composeAPKOperations(&request, &manifest)

		// Then
		require.NoError(t, err)
		operations[0].PackageName = "mutated"
		require.Equal(t, "sentinel", request.APKOperations[0].PackageName)
	})

	t.Run("default operations follow Integer packages", func(t *testing.T) {
		// Given
		request := ComposeRequest{}

		// When
		operations, err := composeAPKOperations(&request, &manifest)

		// Then
		require.NoError(t, err)
		require.Len(t, operations, len(manifest.Packages))
		require.Equal(t, APKUpsert, operations[0].Action)
	})

	t.Run("delta remove is materialized", func(t *testing.T) {
		// Given
		request := ComposeRequest{APKDelta: composeOperationsDelta(t, apkDeltaOperation{Action: APKRemove, Architecture: ArchitectureX8664, PackageName: "retired"})}

		// When
		operations, err := composeAPKOperations(&request, &manifest)

		// Then
		require.NoError(t, err)
		require.Equal(t, []APKOperation{{Action: APKRemove, Architecture: ArchitectureX8664, PackageName: "retired"}}, operations)
	})
}

func TestAPKDelta_rejects_malformed_duplicate_remove_source_and_unknown_actions(t *testing.T) {
	manifest, err := ci.ParseIntegerBatchManifest(composeIntegerManifest(t))
	require.NoError(t, err)
	valid := composeOperationsDelta(t)
	tests := []struct {
		name string
		data []byte
	}{
		{name: "malformed JSON", data: []byte(`{`)},
		{name: "trailing JSON", data: append(append([]byte(nil), valid...), []byte(` {}`)...)},
		{name: "malformed header", data: []byte(`{"format_version":0,"base_sha256":"bad","repository_format":"","key_sha256":"bad","operations":[]}`)},
		{name: "duplicate operation", data: composeOperationsDelta(
			t,
			apkDeltaOperation{Action: APKRemove, Architecture: ArchitectureX8664, PackageName: "same"},
			apkDeltaOperation{Action: APKRemove, Architecture: ArchitectureX8664, PackageName: "same"},
		)},
		{name: "remove carries source", data: composeOperationsDelta(t, apkDeltaOperation{Action: APKRemove, Architecture: ArchitectureX8664, PackageName: "retired", Source: "retired.apk"})},
		{name: "unsupported action", data: composeOperationsDelta(t, apkDeltaOperation{Action: "replace", Architecture: ArchitectureX8664, PackageName: "demo"})},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// When
			_, err := operationsFromDelta(test.data, &manifest)

			// Then
			require.Error(t, err)
		})
	}
}

func composeOperationsDelta(t *testing.T, operations ...apkDeltaOperation) []byte {
	t.Helper()
	data, err := json.Marshal(apkDeltaManifest{
		FormatVersion:    1,
		BaseSHA256:       digestSeed("6"),
		RepositoryFormat: "apk-v2",
		KeySHA256:        digestSeed("7"),
		Operations:       operations,
	})
	require.NoError(t, err)
	return data
}
