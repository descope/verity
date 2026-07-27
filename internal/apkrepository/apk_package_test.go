package apkrepository

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPackageSemanticDigest_ignores_signature_bytes(t *testing.T) {
	// Given semantically identical APKs with different signature streams.
	root := t.TempDir()
	first := filepath.Join(root, "first.apk")
	second := filepath.Join(root, "second.apk")
	writeDigestTestAPK(t, first, "demo", "1.0-r0", "x86_64", "payload", "first-signature")
	writeDigestTestAPK(t, second, "demo", "1.0-r0", "x86_64", "payload", "second-signature")

	// When their semantic digests are calculated.
	firstDigest, firstErr := PackageSemanticDigest(first)
	secondDigest, secondErr := PackageSemanticDigest(second)

	// Then signature churn does not create a package mutation.
	require.NoError(t, firstErr)
	require.NoError(t, secondErr)
	assert.Equal(t, firstDigest, secondDigest)
}

func TestPackageSemanticDigest_changes_with_package_content(t *testing.T) {
	// Given packages with the same identity and different payloads.
	root := t.TempDir()
	first := filepath.Join(root, "first.apk")
	second := filepath.Join(root, "second.apk")
	writeDigestTestAPK(t, first, "demo", "1.0-r0", "x86_64", "first", "signature")
	writeDigestTestAPK(t, second, "demo", "1.0-r0", "x86_64", "second", "signature")

	// When their semantic digests are calculated.
	firstDigest, firstErr := PackageSemanticDigest(first)
	secondDigest, secondErr := PackageSemanticDigest(second)

	// Then payload changes are observable.
	require.NoError(t, firstErr)
	require.NoError(t, secondErr)
	assert.NotEqual(t, firstDigest, secondDigest)
}

func TestInspectPackage_rejects_malformed_APK(t *testing.T) {
	// Given a file that is not an APK archive.
	path := filepath.Join(t.TempDir(), "malformed.apk")
	require.NoError(t, os.WriteFile(path, []byte("not an apk"), 0o644))

	// When package metadata is inspected.
	_, err := inspectPackage(path)

	// Then the malformed boundary input is rejected.
	require.Error(t, err)
	assert.ErrorContains(t, err, "parse APK")
}

func TestInspectPackage_rejects_hostile_data_tar_headers(t *testing.T) {
	tests := []struct {
		name    string
		headers []tar.Header
	}{
		{name: "absolute path", headers: []tar.Header{{Name: "/usr/bin/demo", Typeflag: tar.TypeReg}}},
		{name: "traversal", headers: []tar.Header{{Name: "../escape", Typeflag: tar.TypeReg}}},
		{name: "backslash ambiguity", headers: []tar.Header{{Name: `usr\bin\demo`, Typeflag: tar.TypeReg}}},
		{name: "symlink", headers: []tar.Header{{Name: "usr/bin/demo", Typeflag: tar.TypeSymlink, Linkname: "target"}}},
		{name: "duplicate path", headers: []tar.Header{
			{Name: "usr/bin/demo", Typeflag: tar.TypeReg},
			{Name: "usr/bin/demo", Typeflag: tar.TypeReg},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given an otherwise valid APK with a hostile data archive header.
			path := filepath.Join(t.TempDir(), "hostile.apk")
			writeTestAPKWithDataHeaders(t, path, tt.headers)

			// When the package is inspected before signing.
			_, err := inspectPackage(path)

			// Then the hostile archive is rejected.
			require.ErrorIs(t, err, errInvalidAPK)
		})
	}
}

func TestAPKDataArchiveValidator_enforces_header_and_expansion_bounds(t *testing.T) {
	tests := []struct {
		name      string
		validator apkDataArchiveValidator
		header    tar.Header
	}{
		{name: "empty path", header: tar.Header{Typeflag: tar.TypeReg}},
		{name: "invalid UTF-8", header: tar.Header{Name: string([]byte{0xff}), Typeflag: tar.TypeReg}},
		{name: "NUL path", header: tar.Header{Name: "usr/bin/\x00demo", Typeflag: tar.TypeReg}},
		{name: "noncanonical path", header: tar.Header{Name: "usr//bin/demo", Typeflag: tar.TypeReg}},
		{name: "oversized path", header: tar.Header{Name: strings.Repeat("a", maxAPKPathLength+1), Typeflag: tar.TypeReg}},
		{name: "oversized link", header: tar.Header{Name: "link", Linkname: strings.Repeat("a", maxAPKLinkLength+1), Typeflag: tar.TypeSymlink}},
		{name: "oversized header", header: tar.Header{Name: "file", Typeflag: tar.TypeReg, PAXRecords: map[string]string{"comment": strings.Repeat("a", maxAPKHeaderSize)}}},
		{name: "hardlink", header: tar.Header{Name: "hardlink", Linkname: "target", Typeflag: tar.TypeLink}},
		{name: "character device", header: tar.Header{Name: "device", Typeflag: tar.TypeChar}},
		{name: "block device", header: tar.Header{Name: "device", Typeflag: tar.TypeBlock}},
		{name: "unsupported FIFO", header: tar.Header{Name: "fifo", Typeflag: tar.TypeFifo}},
		{name: "GNU sparse", header: tar.Header{Name: "sparse", Typeflag: tar.TypeGNUSparse}},
		{name: "oversized entry", header: tar.Header{Name: "file", Typeflag: tar.TypeReg, Size: maxAPKEntrySize + 1}},
		{name: "too many entries", validator: apkDataArchiveValidator{entryCount: maxAPKEntryCount}, header: tar.Header{Name: "file", Typeflag: tar.TypeReg}},
		{name: "total expansion", validator: apkDataArchiveValidator{expandedSize: maxAPKExpandedSize}, header: tar.Header{Name: "file", Typeflag: tar.TypeReg, Size: 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given a validator at the stated archive boundary.
			validator := tt.validator

			// When the next header is inspected.
			err := validator.validate(&tt.header)

			// Then the bounded fail-closed policy rejects it.
			require.ErrorIs(t, err, errInvalidAPK)
		})
	}
}

func TestInspectPackage_rejects_nonzero_data_after_tar_EOF(t *testing.T) {
	// Given a valid data entry followed by hidden non-zero bytes after tar EOF.
	path := filepath.Join(t.TempDir(), "trailing.apk")
	var apk bytes.Buffer
	pkgInfo := "pkgname = demo\npkgver = 1.0-r0\narch = x86_64\nsize = 0\n"
	apk.Write(digestTestTarGzip(t, []digestTestTarEntry{{name: ".PKGINFO", contents: pkgInfo}}))
	gzipWriter := gzip.NewWriter(&apk)
	tarWriter := tar.NewWriter(gzipWriter)
	require.NoError(t, tarWriter.WriteHeader(&tar.Header{Name: "usr/bin/demo", Mode: 0o755, Typeflag: tar.TypeReg}))
	require.NoError(t, tarWriter.Close())
	_, err := gzipWriter.Write([]byte("hidden"))
	require.NoError(t, err)
	require.NoError(t, gzipWriter.Close())
	require.NoError(t, os.WriteFile(path, apk.Bytes(), 0o644))

	// When the package is inspected.
	_, err = inspectPackage(path)

	// Then EOF is stable and hidden data is rejected.
	require.ErrorIs(t, err, errInvalidAPK)
}

func TestPackageSemanticDigest_preserves_v1_digest(t *testing.T) {
	// Given the deterministic v1 package fixture.
	path := filepath.Join(t.TempDir(), "demo.apk")
	writeDigestTestAPK(t, path, "demo", "1.0-r0", "x86_64", "payload", "")

	// When its semantic digest is calculated.
	digest, err := PackageSemanticDigest(path)

	// Then archive validation does not change the established digest.
	require.NoError(t, err)
	assert.Equal(t, "sha256:45d38038082cc7e3154b47c5712e2f70991341f19770c076786d26e944b3f0e5", digest)
}

func writeTestAPKWithDataHeaders(t *testing.T, path string, headers []tar.Header) {
	t.Helper()
	var apk bytes.Buffer
	pkgInfo := "pkgname = demo\npkgver = 1.0-r0\narch = x86_64\nsize = 0\n"
	apk.Write(digestTestTarGzip(t, []digestTestTarEntry{{name: ".PKGINFO", contents: pkgInfo}}))
	gzipWriter := gzip.NewWriter(&apk)
	tarWriter := tar.NewWriter(gzipWriter)
	for index := range headers {
		require.NoError(t, tarWriter.WriteHeader(&headers[index]))
	}
	require.NoError(t, tarWriter.Close())
	require.NoError(t, gzipWriter.Close())
	require.NoError(t, os.WriteFile(path, apk.Bytes(), 0o644))
}

type digestTestTarEntry struct {
	name     string
	contents string
}

func writeDigestTestAPK(t *testing.T, filePath, name, version, architecture, payload, signature string) {
	t.Helper()
	var apk bytes.Buffer
	if signature != "" {
		apk.Write(digestTestTarGzip(t, []digestTestTarEntry{{name: ".SIGN.RSA256.test.rsa.pub", contents: signature}}))
	}
	pkgInfo := fmt.Sprintf("pkgname = %s\npkgver = %s\narch = %s\nsize = %d\n", name, version, architecture, len(payload))
	apk.Write(digestTestTarGzip(t, []digestTestTarEntry{{name: ".PKGINFO", contents: pkgInfo}}))
	apk.Write(digestTestTarGzip(t, []digestTestTarEntry{{name: "usr/bin/" + name, contents: payload}}))
	require.NoError(t, os.MkdirAll(filepath.Dir(filePath), 0o755))
	require.NoError(t, os.WriteFile(filePath, apk.Bytes(), 0o644))
}

func digestTestTarGzip(t *testing.T, entries []digestTestTarEntry) []byte {
	t.Helper()
	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	tarWriter := tar.NewWriter(gzipWriter)
	for _, entry := range entries {
		require.NoError(t, tarWriter.WriteHeader(&tar.Header{Name: entry.name, Mode: 0o755, Size: int64(len(entry.contents))}))
		_, err := tarWriter.Write([]byte(entry.contents))
		require.NoError(t, err)
	}
	require.NoError(t, tarWriter.Close())
	require.NoError(t, gzipWriter.Close())
	return buffer.Bytes()
}
