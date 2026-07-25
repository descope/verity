package apkrepository

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyDelta_rejects_duplicate_and_undeclared_mutations(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(*testing.T, *deltaFixture) []DeltaOperation
		wantErr error
	}{
		{
			name: "duplicate key",
			prepare: func(t *testing.T, fixture *deltaFixture) []DeltaOperation {
				t.Helper()
				return []DeltaOperation{
					{Action: "remove", Architecture: "x86_64", PackageName: "demo"},
					{Action: "remove", Architecture: "x86_64", PackageName: "demo"},
				}
			},
			wantErr: errDuplicateDeltaMutation,
		},
		{
			name: "undeclared APK",
			prepare: func(t *testing.T, fixture *deltaFixture) []DeltaOperation {
				t.Helper()
				source := filepath.Join(fixture.packages, "x86_64", "demo.apk")
				writeTestAPK(t, source, "demo", "2.0-r0", "x86_64", "declared", "")
				writeTestAPK(t, filepath.Join(fixture.packages, "x86_64", "extra.apk"), "extra", "1.0-r0", "x86_64", "extra", "")
				digest, err := PackageSemanticDigest(source)
				require.NoError(t, err)
				return []DeltaOperation{{Action: "upsert", Architecture: "x86_64", PackageName: "demo", Source: "x86_64/demo.apk", SHA256: digest}}
			},
			wantErr: errUndeclaredDeltaPackage,
		},
		{
			name: "undeclared APK on remove-only delta",
			prepare: func(t *testing.T, fixture *deltaFixture) []DeltaOperation {
				t.Helper()
				writeTestAPK(t, filepath.Join(fixture.packages, "x86_64", "extra.apk"), "extra", "1.0-r0", "x86_64", "extra", "")
				return []DeltaOperation{{Action: "remove", Architecture: "x86_64", PackageName: "demo"}}
			},
			wantErr: errUndeclaredDeltaPackage,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given an invalid mutation declaration.
			fixture := newDeltaFixture(t)
			manifest := fixture.manifest(t, test.prepare(t, fixture))

			// When the delta is parsed.
			err := ApplyDelta(context.Background(), fixture.options(t, &manifest))

			// Then it is rejected at the manifest boundary.
			require.ErrorIs(t, err, test.wantErr)
		})
	}
}

func TestApplyDelta_rejects_wrong_base_key_and_format(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*DeltaManifest)
		wantErr error
	}{
		{name: "stale base", mutate: func(manifest *DeltaManifest) { manifest.BaseSHA256 = "sha256:" + string(bytes.Repeat([]byte{'0'}, 64)) }, wantErr: errDeltaBaseMismatch},
		{name: "wrong key", mutate: func(manifest *DeltaManifest) { manifest.KeySHA256 = "sha256:" + string(bytes.Repeat([]byte{'1'}, 64)) }, wantErr: errDeltaKeyMismatch},
		{name: "wrong repository format", mutate: func(manifest *DeltaManifest) { manifest.RepositoryFormat = "2" }, wantErr: errDeltaFormatMismatch},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given a delta whose base trust contract does not match publication state.
			fixture := newDeltaFixture(t)
			manifest := fixture.manifest(t, []DeltaOperation{{Action: "remove", Architecture: "x86_64", PackageName: "demo"}})
			test.mutate(&manifest)

			// When the delta is applied.
			err := ApplyDelta(context.Background(), fixture.options(t, &manifest))

			// Then stale or trust-changing input fails closed.
			require.ErrorIs(t, err, test.wantErr)
		})
	}
}

func TestApplyDelta_rejects_invalid_operation_inputs(t *testing.T) {
	tests := []struct {
		name       string
		operations []DeltaOperation
		wantErr    error
	}{
		{name: "unknown architecture", operations: []DeltaOperation{{Action: "remove", Architecture: "loongarch64", PackageName: "demo"}}, wantErr: errUnsupportedArchitecture},
		{name: "missing remove target", operations: []DeltaOperation{{Action: "remove", Architecture: "x86_64", PackageName: "missing"}}, wantErr: errDeltaPackageMissing},
		{name: "missing upsert source", operations: []DeltaOperation{{Action: "upsert", Architecture: "x86_64", PackageName: "demo", Source: "x86_64/missing.apk", SHA256: "sha256:" + string(bytes.Repeat([]byte{'2'}, 64))}}, wantErr: errDeltaPackageMissing},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given an invalid operation.
			fixture := newDeltaFixture(t)
			manifest := fixture.manifest(t, test.operations)

			// When the operation is parsed and resolved.
			err := ApplyDelta(context.Background(), fixture.options(t, &manifest))

			// Then it cannot mutate the repository.
			require.ErrorIs(t, err, test.wantErr)
		})
	}
}

func TestApplyDelta_rejects_tampered_or_mismatched_upsert(t *testing.T) {
	tests := []struct {
		name       string
		write      func(*testing.T, string)
		packageKey string
		wantErr    error
	}{
		{name: "tampered digest", write: func(t *testing.T, path string) { writeTestAPK(t, path, "demo", "2.0-r0", "x86_64", "tampered", "") }, packageKey: "demo", wantErr: errDeltaDigestMismatch},
		{name: "mismatched package name", write: func(t *testing.T, path string) { writeTestAPK(t, path, "other", "2.0-r0", "x86_64", "payload", "") }, packageKey: "demo", wantErr: errDeltaPackageMismatch},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given an upsert whose declared semantic identity is false.
			fixture := newDeltaFixture(t)
			source := filepath.Join(fixture.packages, "x86_64", "candidate.apk")
			writeTestAPK(t, source, "demo", "2.0-r0", "x86_64", "declared", "")
			digest, err := PackageSemanticDigest(source)
			require.NoError(t, err)
			test.write(t, source)
			manifest := fixture.manifest(t, []DeltaOperation{{
				Action: "upsert", Architecture: "x86_64", PackageName: test.packageKey,
				Source: "x86_64/candidate.apk", SHA256: digest,
			}})

			// When the delta is applied.
			err = ApplyDelta(context.Background(), fixture.options(t, &manifest))

			// Then tampering or identity substitution is rejected.
			require.ErrorIs(t, err, test.wantErr)
		})
	}
}

func TestApplyDelta_rejects_malformed_manifest(t *testing.T) {
	// Given a manifest with an unknown field and otherwise plausible content.
	fixture := newDeltaFixture(t)
	manifest := fixture.manifest(t, []DeltaOperation{{Action: "remove", Architecture: "x86_64", PackageName: "demo"}})
	data, err := json.Marshal(manifest)
	require.NoError(t, err)
	data = bytes.Replace(data, []byte(`{"format_version":1`), []byte(`{"unknown":true,"format_version":1`), 1)
	require.NoError(t, os.WriteFile(fixture.manifestAt, data, 0o644))
	options := fixture.options(t, &manifest)
	require.NoError(t, os.WriteFile(fixture.manifestAt, data, 0o644))

	// When the malformed manifest is parsed.
	err = ApplyDelta(context.Background(), options)

	// Then unknown input cannot be silently ignored.
	require.ErrorIs(t, err, errInvalidDeltaManifest)
}

func TestApplyDelta_preserves_output_when_indexing_is_interrupted(t *testing.T) {
	// Given a valid update whose Melange index operation fails.
	fixture := newDeltaFixture(t)
	source := filepath.Join(fixture.packages, "x86_64", "demo.apk")
	writeTestAPK(t, source, "demo", "2.0-r0", "x86_64", "updated", "")
	digest, err := PackageSemanticDigest(source)
	require.NoError(t, err)
	manifest := fixture.manifest(t, []DeltaOperation{{Action: "upsert", Architecture: "x86_64", PackageName: "demo", Source: "x86_64/demo.apk", SHA256: digest}})
	options := fixture.options(t, &manifest)
	options.runner = &fakeCommandRunner{run: func(request command) (commandResult, error) {
		if request.args[0] == "index" {
			return commandResult{exitCode: 9, stderr: []byte("interrupted")}, nil
		}
		return commandResult{}, nil
	}}
	var stdout bytes.Buffer
	options.Stdout = &stdout
	publishedBefore := mustReadFile(t, filepath.Join(fixture.output, "index.html"))

	// When the staged transition is interrupted.
	err = ApplyDelta(context.Background(), options)

	// Then prior output is untouched and no success message is emitted.
	require.Error(t, err)
	assert.Equal(t, publishedBefore, mustReadFile(t, filepath.Join(fixture.output, "index.html")))
	assert.NoDirExists(t, filepath.Join(fixture.output, "x86_64"))
	assert.Empty(t, stdout.String())
}
