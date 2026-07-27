package buildmetadata

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testSourceSHA = "0123456789012345678901234567890123456789"
	testBuildKey  = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
)

func TestPackageAndVerifyArtifact_accepts_only_the_canonical_three_file_payload(t *testing.T) {
	// Given one regular Verity binary in an empty artifact directory.
	directory := newArtifactDirectory(t)

	// When the artifact is packaged and verified against its exact identity.
	manifest, err := PackageArtifact(PackageOptions{
		Directory: directory, SourceSHA: testSourceSHA, BuildKey: testBuildKey, GoVersion: "go1.26.5",
	})
	require.NoError(t, err)
	verified, err := VerifyArtifact(VerifyOptions{Directory: directory, SourceSHA: testSourceSHA, BuildKey: testBuildKey})

	// Then the manifest, checksum, and exact file set are accepted.
	require.NoError(t, err)
	assert.Equal(t, manifest, verified.Manifest)
	assert.Equal(t, filepath.Join(directory, BinaryName), verified.BinaryPath)
	entries, err := os.ReadDir(directory)
	require.NoError(t, err)
	assert.Len(t, entries, 3)
}

func TestVerifyArtifact_rejects_identity_checksum_and_canonical_JSON_mutations(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, string)
		verify VerifyOptions
	}{
		{name: "wrong source", verify: VerifyOptions{SourceSHA: strings.Repeat("b", 40), BuildKey: testBuildKey}},
		{name: "wrong build key", verify: VerifyOptions{SourceSHA: testSourceSHA, BuildKey: strings.Repeat("b", 64)}},
		{name: "wrong checksum", mutate: func(t *testing.T, directory string) {
			require.NoError(t, os.WriteFile(filepath.Join(directory, ChecksumName), []byte(strings.Repeat("0", 64)+"  verity\n"), 0o600))
		}, verify: VerifyOptions{SourceSHA: testSourceSHA, BuildKey: testBuildKey}},
		{name: "checksum trailing data", mutate: func(t *testing.T, directory string) {
			path := filepath.Join(directory, ChecksumName)
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(path, append(data, []byte("trailing\n")...), 0o600))
		}, verify: VerifyOptions{SourceSHA: testSourceSHA, BuildKey: testBuildKey}},
		{name: "noncanonical build JSON", mutate: func(t *testing.T, directory string) {
			path := filepath.Join(directory, ManifestName)
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(path, append(data, '\n'), 0o600))
		}, verify: VerifyOptions{SourceSHA: testSourceSHA, BuildKey: testBuildKey}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given a canonical packaged artifact with one hostile mutation.
			directory := packagedArtifact(t)
			if test.mutate != nil {
				test.mutate(t, directory)
			}
			options := test.verify
			options.Directory = directory

			// When exact verification runs.
			_, err := VerifyArtifact(options)

			// Then the mutation is rejected before activation.
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrInvalidArtifact)
		})
	}
}

func TestVerifyArtifact_rejects_path_symlink_archive_and_trailing_file_mutations(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, string) string
	}{
		{name: "artifact directory symlink", mutate: func(t *testing.T, directory string) string {
			alias := filepath.Join(t.TempDir(), "artifact-link")
			require.NoError(t, os.Symlink(directory, alias))
			return alias
		}},
		{name: "binary symlink", mutate: func(t *testing.T, directory string) string {
			binary := filepath.Join(directory, BinaryName)
			target := filepath.Join(t.TempDir(), "outside-verity")
			require.NoError(t, os.WriteFile(target, []byte("outside"), 0o600))
			require.NoError(t, os.Remove(binary))
			require.NoError(t, os.Symlink(target, binary))
			return directory
		}},
		{name: "nested path", mutate: func(t *testing.T, directory string) string {
			require.NoError(t, os.Mkdir(filepath.Join(directory, "nested"), 0o700))
			return directory
		}},
		{name: "archive file", mutate: func(t *testing.T, directory string) string {
			require.NoError(t, os.WriteFile(filepath.Join(directory, "verity.zip"), []byte("archive"), 0o600))
			return directory
		}},
		{name: "trailing file", mutate: func(t *testing.T, directory string) string {
			require.NoError(t, os.WriteFile(filepath.Join(directory, "extra"), []byte("extra"), 0o600))
			return directory
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given a canonical artifact whose path shape is mutated.
			directory := test.mutate(t, packagedArtifact(t))

			// When exact verification runs.
			_, err := VerifyArtifact(VerifyOptions{Directory: directory, SourceSHA: testSourceSHA, BuildKey: testBuildKey})

			// Then no mutated path or trailing payload is trusted.
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrInvalidArtifact)
		})
	}
}

func TestActivateArtifact_chmods_only_after_reverification(t *testing.T) {
	// Given one valid non-executable artifact.
	directory := packagedArtifact(t)
	binary := filepath.Join(directory, BinaryName)
	require.NoError(t, os.Chmod(binary, 0o600))

	// When activation performs a final exact verification.
	verified, err := ActivateArtifact(VerifyOptions{Directory: directory, SourceSHA: testSourceSHA, BuildKey: testBuildKey})

	// Then the exact binary becomes executable and is returned to the caller.
	require.NoError(t, err)
	assert.Equal(t, binary, verified.BinaryPath)
	info, err := os.Stat(binary)
	require.NoError(t, err)
	assert.NotZero(t, info.Mode().Perm()&0o111)
}

func TestActivateArtifact_leaves_tampered_binary_non_executable(t *testing.T) {
	// Given a packaged artifact with a tampered checksum and non-executable binary.
	directory := packagedArtifact(t)
	binary := filepath.Join(directory, BinaryName)
	require.NoError(t, os.Chmod(binary, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(directory, ChecksumName), []byte(strings.Repeat("0", 64)+"  verity\n"), 0o600))

	// When activation is attempted.
	_, err := ActivateArtifact(VerifyOptions{Directory: directory, SourceSHA: testSourceSHA, BuildKey: testBuildKey})

	// Then activation fails without granting execute permission.
	require.Error(t, err)
	info, statErr := os.Stat(binary)
	require.NoError(t, statErr)
	assert.Zero(t, info.Mode().Perm()&0o111)
}

func packagedArtifact(t *testing.T) string {
	t.Helper()
	directory := newArtifactDirectory(t)
	_, err := PackageArtifact(PackageOptions{
		Directory: directory, SourceSHA: testSourceSHA, BuildKey: testBuildKey, GoVersion: "go1.26.5",
	})
	require.NoError(t, err)
	return directory
}

func newArtifactDirectory(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(directory, BinaryName), []byte("verity-binary"), 0o600))
	return directory
}
