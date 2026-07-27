package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/buildmetadata"
)

const (
	testActionSourceSHA = "0123456789012345678901234567890123456789"
	testActionBuildKey  = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
)

type zipFixtureEntry struct {
	name string
	data []byte
	mode os.FileMode
}

func TestExtractArtifact_accepts_only_canonical_three_file_archive(t *testing.T) {
	// Given one exact current-run archive with canonical metadata and checksum.
	options := canonicalExtractFixture(t)

	// When the archive is verified and extracted.
	verified, err := extractArtifact(&options)

	// Then exactly three non-executable regular files are accepted.
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(options.ArtifactDirectory, binaryName), verified.BinaryPath)
	entries, err := os.ReadDir(options.ArtifactDirectory)
	require.NoError(t, err)
	assert.Len(t, entries, 3)
	info, err := os.Lstat(verified.BinaryPath)
	require.NoError(t, err)
	assert.Zero(t, info.Mode().Perm()&0o111)
}

func TestExtractArtifact_rejects_hostile_archive_and_identity_mutations(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, []zipFixtureEntry, *extractOptions) []zipFixtureEntry
	}{
		{name: "duplicate binary", mutate: func(_ *testing.T, entries []zipFixtureEntry, _ *extractOptions) []zipFixtureEntry {
			return append(entries, entries[0])
		}},
		{name: "path traversal", mutate: func(_ *testing.T, entries []zipFixtureEntry, _ *extractOptions) []zipFixtureEntry {
			entries[0].name = "../verity"
			return entries
		}},
		{name: "backslash path", mutate: func(_ *testing.T, entries []zipFixtureEntry, _ *extractOptions) []zipFixtureEntry {
			entries[0].name = `nested\verity`
			return entries
		}},
		{name: "symlink entry", mutate: func(_ *testing.T, entries []zipFixtureEntry, _ *extractOptions) []zipFixtureEntry {
			entries[0].mode = os.ModeSymlink | 0o777
			return entries
		}},
		{name: "nested path", mutate: func(_ *testing.T, entries []zipFixtureEntry, _ *extractOptions) []zipFixtureEntry {
			entries[0].name = "nested/verity"
			return entries
		}},
		{name: "trailing archive", mutate: func(_ *testing.T, entries []zipFixtureEntry, _ *extractOptions) []zipFixtureEntry {
			return append(entries, zipFixtureEntry{name: "verity.zip", data: []byte("archive"), mode: 0o600})
		}},
		{name: "wrong checksum", mutate: func(_ *testing.T, entries []zipFixtureEntry, _ *extractOptions) []zipFixtureEntry {
			entries[1].data = []byte(strings.Repeat("0", 64) + "  verity\n")
			return entries
		}},
		{name: "wrong source", mutate: func(t *testing.T, entries []zipFixtureEntry, _ *extractOptions) []zipFixtureEntry {
			entries[2].data = fakeBuildJSON(t, artifactIdentity{SourceSHA: strings.Repeat("b", 40), BuildKey: testActionBuildKey})
			return entries
		}},
		{name: "wrong build key", mutate: func(t *testing.T, entries []zipFixtureEntry, _ *extractOptions) []zipFixtureEntry {
			entries[2].data = fakeBuildJSON(t, artifactIdentity{SourceSHA: testActionSourceSHA, BuildKey: strings.Repeat("b", 64)})
			return entries
		}},
		{name: "noncanonical build JSON", mutate: func(_ *testing.T, entries []zipFixtureEntry, _ *extractOptions) []zipFixtureEntry {
			entries[2].data = append(entries[2].data, '\n')
			return entries
		}},
		{name: "wrong archive digest", mutate: func(_ *testing.T, entries []zipFixtureEntry, options *extractOptions) []zipFixtureEntry {
			options.ArtifactDigest = "sha256:" + strings.Repeat("0", 64)
			return entries
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given one hostile archive mutation.
			identity := artifactIdentity{SourceSHA: testActionSourceSHA, BuildKey: testActionBuildKey}
			entries := canonicalZipEntries(t, identity)
			options := newExtractOptions(t, identity)
			entries = test.mutate(t, entries, &options)
			writeArchiveFixture(t, options.DownloadDirectory, entries, &options)

			// When exact extraction is attempted.
			_, err := extractArtifact(&options)

			// Then the archive is rejected before activation.
			require.Error(t, err)
			assert.ErrorIs(t, err, errUntrustedArtifact)
		})
	}
}

func TestExtractArtifact_rejects_symlink_download_directory(t *testing.T) {
	// Given a canonical archive reached through a symlinked download directory.
	options := canonicalExtractFixture(t)
	alias := filepath.Join(t.TempDir(), "download-link")
	require.NoError(t, os.Symlink(options.DownloadDirectory, alias))
	options.DownloadDirectory = alias

	// When extraction is attempted.
	_, err := extractArtifact(&options)

	// Then the mutable directory alias is rejected.
	require.Error(t, err)
	assert.ErrorIs(t, err, errUntrustedArtifact)
}

func canonicalExtractFixture(t *testing.T) extractOptions {
	t.Helper()
	identity := artifactIdentity{SourceSHA: testActionSourceSHA, BuildKey: testActionBuildKey}
	options := newExtractOptions(t, identity)
	writeArchiveFixture(t, options.DownloadDirectory, canonicalZipEntries(t, identity), &options)
	return options
}

func newExtractOptions(t *testing.T, identity artifactIdentity) extractOptions {
	t.Helper()
	return extractOptions{
		DownloadDirectory: filepath.Join(t.TempDir(), "download"),
		ArtifactDirectory: filepath.Join(t.TempDir(), "artifact"),
		Identity:          identity,
	}
}

func canonicalZipEntries(t *testing.T, identity artifactIdentity) []zipFixtureEntry {
	t.Helper()
	binary := []byte("fake-verity-binary")
	digest := sha256.Sum256(binary)
	return []zipFixtureEntry{
		{name: binaryName, data: binary, mode: 0o600},
		{name: checksumName, data: []byte(hex.EncodeToString(digest[:]) + "  verity\n"), mode: 0o600},
		{name: buildJSONName, data: fakeBuildJSON(t, identity), mode: 0o600},
	}
}

func fakeBuildJSON(t *testing.T, identity artifactIdentity) []byte {
	t.Helper()
	data, err := buildmetadata.MarshalInfo(buildmetadata.Info{
		Version: buildVersion, SourceSHA: identity.SourceSHA, BuildKey: identity.BuildKey,
		GoVersion: "go1.26.5", GOOS: "linux", GOARCH: "amd64", CGOEnabled: "0",
		BuildFlags: append([]string(nil), runtimeBuildFlags...), Dirty: new(bool),
		VCSStatus: buildmetadata.CleanVCSStatus, BuildStatus: buildmetadata.BuiltStatus,
	})
	require.NoError(t, err)
	return data
}

func writeArchiveFixture(t *testing.T, directory string, entries []zipFixtureEntry, options *extractOptions) {
	t.Helper()
	require.NoError(t, os.MkdirAll(directory, 0o700))
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, entry := range entries {
		header := &zip.FileHeader{Name: entry.name, Method: zip.Store}
		header.SetMode(entry.mode)
		file, err := writer.CreateHeader(header)
		require.NoError(t, err)
		_, err = file.Write(entry.data)
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())
	archivePath := filepath.Join(directory, "artifact.zip")
	require.NoError(t, os.WriteFile(archivePath, buffer.Bytes(), 0o600))
	digest := sha256.Sum256(buffer.Bytes())
	if options.ArtifactDigest == "" {
		options.ArtifactDigest = "sha256:" + hex.EncodeToString(digest[:])
	}
}
