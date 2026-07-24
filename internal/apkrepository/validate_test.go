package apkrepository

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidate_accepts_guarded_empty_repository(t *testing.T) {
	// Given an assembled repository with the explicit empty marker.
	repository := t.TempDir()
	writeTestFile(t, filepath.Join(repository, ".no-apks-found"), "empty")
	var stdout bytes.Buffer

	// When the layout is validated.
	err := Validate(context.Background(), &ValidateOptions{RepositoryDir: repository, Stdout: &stdout})

	// Then the guarded empty state passes.
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "guarded empty repository marker found")
}

func TestValidate_rejects_packages_outside_direct_architecture_directories(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		message string
	}{
		{name: "repository root", path: "bad.apk", message: "APK files must live under architecture directories"},
		{name: "nested directory", path: filepath.Join("x86_64", "nested", "bad.apk"), message: "directly under architecture directories"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given a package at an invalid depth.
			repository := t.TempDir()
			writeTestFile(t, filepath.Join(repository, test.path), "package")

			// When the layout is validated.
			err := Validate(context.Background(), &ValidateOptions{RepositoryDir: repository})

			// Then the invalid location is rejected.
			require.Error(t, err)
			assert.ErrorContains(t, err, test.message)
		})
	}
}

func TestValidate_requires_supported_architecture_and_index(t *testing.T) {
	tests := []struct {
		name    string
		arch    string
		index   bool
		message string
	}{
		{name: "unsupported architecture", arch: "not-an-arch", index: true, message: "unsupported architecture directory"},
		{name: "missing index", arch: "x86_64", index: false, message: "missing APKINDEX.tar.gz"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given a repository with one structurally invalid architecture.
			repository := t.TempDir()
			writeTestFile(t, filepath.Join(repository, test.arch, "demo.apk"), "package")
			if test.index {
				writeTestIndex(t, filepath.Join(repository, test.arch, "APKINDEX.tar.gz"), "APKINDEX")
			}

			// When the layout is validated.
			err := Validate(context.Background(), &ValidateOptions{RepositoryDir: repository})

			// Then validation fails with the structural reason.
			require.Error(t, err)
			assert.ErrorContains(t, err, test.message)
		})
	}
}

func TestValidate_requires_RSA256_signature_and_matching_root_key(t *testing.T) {
	tests := []struct {
		name      string
		signature string
		rootKey   string
		message   string
	}{
		{name: "legacy signature", signature: ".SIGN.RSA.verity.rsa.pub", rootKey: "verity.rsa.pub", message: "missing RSA256 signature"},
		{name: "missing matching key", signature: ".SIGN.RSA256.verity.rsa.pub", rootKey: "other.rsa.pub", message: "no matching root public key"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given a signed-looking repository with invalid trust metadata.
			repository := t.TempDir()
			writeTestFile(t, filepath.Join(repository, "x86_64", "demo.apk"), "package")
			writeTestIndex(t, filepath.Join(repository, "x86_64", "APKINDEX.tar.gz"), test.signature, "APKINDEX")
			if test.rootKey != "" {
				writeTestFile(t, filepath.Join(repository, test.rootKey), "key")
			}

			// When signature metadata is required.
			err := Validate(context.Background(), &ValidateOptions{RepositoryDir: repository, RequireSignature: true})

			// Then only the RSA256 naming contract with a matching root key passes.
			require.Error(t, err)
			assert.ErrorContains(t, err, test.message)
		})
	}
}

func TestValidate_accepts_matching_RSA256_signature_name(t *testing.T) {
	// Given a valid repository layout and matching root key name.
	repository := t.TempDir()
	writeTestFile(t, filepath.Join(repository, "x86_64", "demo.apk"), "package")
	writeTestIndex(t, filepath.Join(repository, "x86_64", "APKINDEX.tar.gz"), ".SIGN.RSA256.verity.rsa.pub", "APKINDEX")
	writeTestFile(t, filepath.Join(repository, "verity.rsa.pub"), "key")

	// When signature metadata is validated.
	err := Validate(context.Background(), &ValidateOptions{RepositoryDir: repository, RequireSignature: true})

	// Then the repository passes without requiring external apk tooling.
	require.NoError(t, err)
}
