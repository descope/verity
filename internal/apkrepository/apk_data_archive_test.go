package apkrepository

import (
	"archive/tar"
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAPKDataArchiveValidator_accepts_NUL_regular_file_type(t *testing.T) {
	// Given a valid regular file encoded with the legacy tar NUL type flag.
	header := &tar.Header{Name: "usr/bin/demo", Typeflag: 0}
	validator := apkDataArchiveValidator{}

	// When the archive entry is validated.
	err := validator.validate(header)

	// Then compatibility is preserved without accepting any additional entry types.
	require.NoError(t, err)
}

func TestAPKDataArchiveValidator_preserves_extended_attribute_header_budget(t *testing.T) {
	// Given a PAX xattr whose mirrored tar representation exceeds the header budget.
	var archive bytes.Buffer
	writer := tar.NewWriter(&archive)
	require.NoError(t, writer.WriteHeader(&tar.Header{
		Name:     "usr/bin/demo",
		Typeflag: tar.TypeReg,
		PAXRecords: map[string]string{
			paxXattrNamespace + "user.test": strings.Repeat("a", maxAPKHeaderSize/2),
		},
	}))
	require.NoError(t, writer.Close())
	reader := tar.NewReader(bytes.NewReader(archive.Bytes()))
	header, err := reader.Next()
	require.NoError(t, err)
	validator := apkDataArchiveValidator{}

	// When the decoded archive entry is validated.
	err = validator.validate(header)

	// Then the existing fail-closed header budget remains enforced.
	require.ErrorIs(t, err, errInvalidAPK)
	require.ErrorContains(t, err, "data header exceeds")
}
