package sitepublication

import (
	"archive/tar"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateArchive_rejects_noncanonical_bytes_with_matching_digest(t *testing.T) {
	// Given a coherent site packed with noncanonical owner metadata.
	plan, _, site := assembledFixture(t)
	archive := filepath.Join(filepath.Dir(site), "noncanonical.tar")
	file, err := os.Create(archive)
	require.NoError(t, err)
	writer := tar.NewWriter(file)
	files, err := listTreeFiles(site)
	require.NoError(t, err)
	for _, entry := range files {
		info, err := os.Stat(entry.path)
		require.NoError(t, err)
		header := &tar.Header{
			Name: entry.relative, Mode: int64(entry.mode.Perm()), Size: info.Size(), Typeflag: tar.TypeReg,
			ModTime: archiveEpoch, Uid: 0, Gid: 0, Uname: "nobody", Gname: "nobody", Format: tar.FormatUSTAR,
		}
		require.NoError(t, writer.WriteHeader(header))
		input, err := os.Open(entry.path)
		require.NoError(t, err)
		_, copyErr := io.Copy(writer, input)
		require.NoError(t, errors.Join(copyErr, input.Close()))
	}
	require.NoError(t, writer.Close())
	require.NoError(t, file.Close())
	digest, err := fileDigest(archive)
	require.NoError(t, err)

	// When validation receives the matching digest for those bytes.
	_, err = ValidateArchive(archive, digest, plan.ManifestDigest)

	// Then deterministic-format validation still rejects the legacy variant.
	require.ErrorIs(t, err, ErrInvalidArchive)
}
