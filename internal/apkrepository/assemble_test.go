package apkrepository

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAssemble_writes_marker_when_sources_have_no_packages(t *testing.T) {
	// Given an empty source and an output containing site documentation.
	root := t.TempDir()
	source := filepath.Join(root, "source")
	output := filepath.Join(root, "site", "dist", "apk")
	require.NoError(t, os.MkdirAll(source, 0o755))
	writeTestFile(t, filepath.Join(output, "index.html"), "docs")
	var stdout bytes.Buffer

	// When the repository is assembled.
	err := Assemble(context.Background(), &AssembleOptions{
		OutputDir: output,
		Sources:   []string{source},
		Stdout:    &stdout,
	})

	// Then the guarded marker is written without disturbing the documentation.
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "No APK files found")
	assert.FileExists(t, filepath.Join(output, ".no-apks-found"))
	assert.FileExists(t, filepath.Join(output, "index.html"))
}

func TestAssemble_cleans_only_managed_artifacts(t *testing.T) {
	// Given stale repository state and an unrelated overlapping public key.
	root := t.TempDir()
	source := filepath.Join(root, "source")
	output := filepath.Join(root, "output")
	require.NoError(t, os.MkdirAll(source, 0o755))
	writeTestFile(t, filepath.Join(output, "x86_64", "old.apk"), "stale")
	writeTestFile(t, filepath.Join(output, "verity.rsa.pub"), "managed")
	writeTestFile(t, filepath.Join(output, "rotation.rsa.pub"), "unrelated")
	writeTestFile(t, filepath.Join(output, "index.html"), "docs")

	// When an empty repository is assembled.
	err := Assemble(context.Background(), &AssembleOptions{OutputDir: output, Sources: []string{source}})

	// Then only Verity-managed state is replaced.
	require.NoError(t, err)
	assert.NoDirExists(t, filepath.Join(output, "x86_64"))
	assert.NoFileExists(t, filepath.Join(output, "verity.rsa.pub"))
	assert.FileExists(t, filepath.Join(output, "rotation.rsa.pub"))
	assert.FileExists(t, filepath.Join(output, "index.html"))
}

func TestAssemble_rejects_conflicting_duplicate_destinations(t *testing.T) {
	// Given two different packages that map to the same repository path.
	root := t.TempDir()
	first := filepath.Join(root, "first")
	second := filepath.Join(root, "second")
	writeTestFile(t, filepath.Join(first, "x86_64", "demo.apk"), "first")
	writeTestFile(t, filepath.Join(second, "x86_64", "demo.apk"), "second")

	// When the repository is assembled.
	err := Assemble(context.Background(), &AssembleOptions{
		OutputDir: filepath.Join(root, "output"),
		Sources:   []string{first, second},
	})

	// Then ambiguous publication is rejected before index creation.
	require.Error(t, err)
	assert.ErrorContains(t, err, "duplicate APK destination x86_64/demo.apk")
}

func TestAssemble_rejects_unsafe_paths_and_key_names(t *testing.T) {
	tests := []struct {
		name    string
		options AssembleOptions
		message string
	}{
		{name: "parent traversal", options: AssembleOptions{OutputDir: "../outside"}, message: "unsafe output directory"},
		{name: "non RSA key", options: AssembleOptions{OutputDir: "output", KeyName: "verity.ec"}, message: "key name must end with .rsa"},
		{name: "key traversal", options: AssembleOptions{OutputDir: "output", KeyName: "../verity.rsa"}, message: "unsafe key name"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// When invalid boundary input is parsed.
			err := Assemble(context.Background(), &test.options)

			// Then assembly fails before touching the filesystem.
			require.Error(t, err)
			assert.ErrorContains(t, err, test.message)
		})
	}
}

func TestAssemble_rejects_package_outside_supported_architecture(t *testing.T) {
	// Given an APK under an unsupported architecture directory.
	root := t.TempDir()
	source := filepath.Join(root, "source")
	writeTestFile(t, filepath.Join(source, "loongarch64", "demo.apk"), "package")

	// When the repository is assembled.
	err := Assemble(context.Background(), &AssembleOptions{
		OutputDir: filepath.Join(root, "output"),
		Sources:   []string{source},
	})

	// Then the package cannot silently enter an unserved architecture.
	require.Error(t, err)
	assert.ErrorContains(t, err, "could not determine APK architecture")
}
