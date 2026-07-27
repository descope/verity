package sitepublication

import (
	"errors"
	"hash"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errSentinelDigestWrite = errors.New("sentinel digest write failure")

func TestClassifySignerRepositoryPath_accepts_only_managed_repository_outputs(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		include bool
		wantErr bool
	}{
		{name: "repository format", path: "repository-format", include: true},
		{name: "public key", path: "verity.rsa.pub", include: true},
		{name: "empty marker", path: ".no-apks-found", include: false},
		{name: "x86 index", path: "x86_64/APKINDEX.tar.gz", include: true},
		{name: "arm package", path: "aarch64/demo.apk", include: true},
		{name: "unexpected root file", path: "README", wantErr: true},
		{name: "unexpected architecture", path: "ppc64le/demo.apk", wantErr: true},
		{name: "unexpected artifact", path: "x86_64/index.json", wantErr: true},
		{name: "nested path", path: "x86_64/nested/demo.apk", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// When
			include, err := classifySignerRepositoryPath(test.path)

			// Then
			if test.wantErr {
				require.ErrorIs(t, err, ErrSignerExecution)
				assert.False(t, include)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.include, include)
		})
	}
}

func TestSignerRepositoryDigest_is_deterministic_and_ignores_empty_marker(t *testing.T) {
	// Given
	root := t.TempDir()
	writeSiteFile(t, root, "repository-format", "v1\n")
	writeSiteFile(t, root, "verity.rsa.pub", "sentinel-public-key")
	writeSiteFile(t, root, "x86_64/APKINDEX.tar.gz", "sentinel-x86-index")
	writeSiteFile(t, root, "x86_64/demo.apk", "sentinel-x86-apk")
	writeSiteFile(t, root, "aarch64/demo.apk", "sentinel-arm-apk")
	writeSiteFile(t, root, ".no-apks-found", "ignored-one")

	// When
	first, err := signerRepositoryDigest(root)
	require.NoError(t, err)
	writeSiteFile(t, root, ".no-apks-found", "ignored-two")
	second, err := signerRepositoryDigest(root)
	require.NoError(t, err)
	writeSiteFile(t, root, "x86_64/demo.apk", "changed-managed-apk")
	changed, err := signerRepositoryDigest(root)

	// Then
	require.NoError(t, err)
	assert.Equal(t, first, second)
	assert.NotEqual(t, first, changed)
	assert.True(t, digestPattern.MatchString(string(first)))
}

func TestSignerRepositoryDigest_rejects_unexpected_output_path(t *testing.T) {
	// Given
	root := t.TempDir()
	writeSiteFile(t, root, "repository-format", "v1\n")
	writeSiteFile(t, root, filepath.Join("x86_64", "unexpected.json"), "sentinel")

	// When
	digest, err := signerRepositoryDigest(root)

	// Then
	require.ErrorIs(t, err, ErrSignerExecution)
	assert.Empty(t, digest)
}

func TestWriteSignerDigestField_propagates_length_and_value_write_failures(t *testing.T) {
	tests := []struct {
		name   string
		failAt int
	}{
		{name: "length", failAt: 1},
		{name: "value", failAt: 2},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given
			digest := &failingSignerHash{failAt: test.failAt}

			// When
			err := writeSignerDigestField(digest, "x86_64/demo.apk")

			// Then
			require.ErrorIs(t, err, errSentinelDigestWrite)
		})
	}
}

type failingSignerHash struct {
	calls  int
	failAt int
}

var _ hash.Hash = (*failingSignerHash)(nil)

func (digest *failingSignerHash) Write(data []byte) (int, error) {
	digest.calls++
	if digest.calls == digest.failAt {
		return 0, errSentinelDigestWrite
	}
	return len(data), nil
}

func (*failingSignerHash) Sum(data []byte) []byte { return data }
func (*failingSignerHash) Reset()                 {}
func (*failingSignerHash) Size() int              { return 32 }
func (*failingSignerHash) BlockSize() int         { return 64 }
