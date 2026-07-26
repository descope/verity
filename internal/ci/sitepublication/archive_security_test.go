package sitepublication

import (
	"archive/tar"
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPackSite_rejects_archive_parent_symlink_into_site(t *testing.T) {
	// Given an archive parent alias that resolves inside the assembled site.
	_, _, site := assembledFixture(t)
	alias := filepath.Join(filepath.Dir(site), "archive-alias")
	require.NoError(t, os.Symlink(site, alias))
	archive := filepath.Join(alias, "artifact.tar")

	// When packing targets the lexical external alias.
	_, err := PackSite(site, archive)

	// Then finalization cannot add an undeclared archive through the link.
	require.ErrorIs(t, err, ErrInvalidArchive)
	assert.NoFileExists(t, filepath.Join(site, "artifact.tar"))
}

func TestPackSite_rejects_site_root_with_symlinked_path_component(t *testing.T) {
	// Given a site reached through a symlinked ancestor rather than its canonical root.
	_, _, site := assembledFixture(t)
	alias := filepath.Join(filepath.Dir(filepath.Dir(site)), "site-parent-alias")
	require.NoError(t, os.Symlink(filepath.Dir(site), alias))
	aliasedSite := filepath.Join(alias, filepath.Base(site))

	// When the aliased tree is packed.
	_, err := PackSite(aliasedSite, filepath.Join(filepath.Dir(site), "artifact.tar"))

	// Then no path component may be resolved through a symlink.
	require.ErrorIs(t, err, ErrInvalidAssembly)
}

func TestPackSite_rejects_internal_and_external_file_symlinks(t *testing.T) {
	tests := []struct {
		name   string
		target func(string) string
	}{
		{name: "internal", target: func(site string) string { return filepath.Join(site, "index.html") }},
		{name: "external", target: func(string) string {
			path := filepath.Join(t.TempDir(), "outside")
			require.NoError(t, os.WriteFile(path, []byte("outside"), 0o600))
			return path
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given a symlink entry in an otherwise coherent site.
			_, _, site := assembledFixture(t)
			require.NoError(t, os.Symlink(test.target(site), filepath.Join(site, "linked")))

			// When the tree is packed.
			_, err := PackSite(site, filepath.Join(filepath.Dir(site), "artifact.tar"))

			// Then links are rejected whether they stay inside or escape the tree.
			require.ErrorIs(t, err, ErrInvalidAssembly)
		})
	}
}

func TestPackSite_rejects_hard_linked_declared_file(t *testing.T) {
	// Given a declared site path replaced by a same-byte external hardlink.
	_, _, site := assembledFixture(t)
	outside := filepath.Join(filepath.Dir(site), "outside-catalog")
	require.NoError(t, os.WriteFile(outside, []byte("catalog"), 0o644))
	inside := filepath.Join(site, "catalog", "keep.json")
	require.NoError(t, os.Remove(inside))
	require.NoError(t, os.Link(outside, inside))

	// When archive packing opens the declared path.
	_, err := PackSite(site, filepath.Join(filepath.Dir(site), "artifact.tar"))

	// Then an external inode alias cannot mutate packed bytes behind validation.
	require.ErrorIs(t, err, ErrInvalidAssembly)
}

func TestPublishSiteSnapshot_rejects_swap_after_validation(t *testing.T) {
	// Given an exact secure snapshot whose source is swapped before publication.
	_, _, site := assembledFixture(t)
	snapshot, err := captureSiteSnapshot(site)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, snapshot.Close()) })
	writeSiteFile(t, site, "catalog/keep.json", "swapped-after-validation")
	archive := filepath.Join(filepath.Dir(site), "artifact.tar")

	// When the validated snapshot is about to become the deploy artifact.
	_, err = publishSiteSnapshot(snapshot, archive)

	// Then exact source-byte revalidation blocks the stale snapshot.
	require.ErrorIs(t, err, ErrUndeclaredMutation)
	assert.NoFileExists(t, archive)
}

func TestExtractSiteArchive_rejects_parent_traversal(t *testing.T) {
	// Given a tar entry that escapes its extraction root.
	var archive bytes.Buffer
	writer := tar.NewWriter(&archive)
	header := &tar.Header{
		Name: "../escape", Mode: 0o644, Size: 1, Typeflag: tar.TypeReg,
		ModTime: archiveEpoch, Uid: 0, Gid: 0, Format: tar.FormatUSTAR,
	}
	require.NoError(t, writer.WriteHeader(header))
	_, err := writer.Write([]byte("x"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	parent := t.TempDir()
	output := filepath.Join(parent, "site")

	// When the hostile archive is extracted.
	_, err = ExtractSiteArchive(bytes.NewReader(archive.Bytes()), output)

	// Then traversal is rejected and no outside file is created.
	require.ErrorIs(t, err, ErrInvalidArchive)
	assert.NoFileExists(t, filepath.Join(parent, "escape"))
}

func TestExtractSiteArchive_rejects_symlink_escape_beneath_output_root(t *testing.T) {
	// Given an existing extraction root containing a symlink to an outside directory.
	output := t.TempDir()
	outside := t.TempDir()
	require.NoError(t, os.Symlink(outside, filepath.Join(output, "catalog")))
	var archive bytes.Buffer
	writer := tar.NewWriter(&archive)
	header := &tar.Header{
		Name: "catalog/escape", Mode: 0o644, Size: 1, Typeflag: tar.TypeReg,
		ModTime: archiveEpoch, Uid: 0, Gid: 0, Format: tar.FormatUSTAR,
	}
	require.NoError(t, writer.WriteHeader(header))
	_, err := writer.Write([]byte("x"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	// When the hostile archive is extracted.
	_, err = ExtractSiteArchive(bytes.NewReader(archive.Bytes()), output)

	// Then the symlink cannot redirect an archive write outside the root.
	require.ErrorIs(t, err, ErrInvalidArchive)
	assert.NoFileExists(t, filepath.Join(outside, "escape"))
}
