package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
)

type goListFixtureModule struct {
	Main bool
}

type goListFixturePackage struct {
	Dir             string
	GoFiles         []string
	CgoFiles        []string
	CFiles          []string
	CXXFiles        []string
	MFiles          []string
	HFiles          []string
	FFiles          []string
	SFiles          []string
	SwigFiles       []string
	SwigCXXFiles    []string
	SysoFiles       []string
	EmbedFiles      []string
	TestGoFiles     []string
	XTestGoFiles    []string
	TestEmbedFiles  []string
	XTestEmbedFiles []string
	Module          *goListFixtureModule
}

func TestDecodeProductionDependencyFiles_includes_all_static_build_file_classes(t *testing.T) {
	// Given a main-module package containing every file class reported by go list for production builds.
	root := t.TempDir()
	packageDir := filepath.Join(root, "internal", "dep")
	input := encodeGoListFixtures(t, goListFixturePackage{
		Dir: packageDir, Module: &goListFixtureModule{Main: true},
		GoFiles: []string{"dep.go"}, CgoFiles: []string{"bridge.go"},
		CFiles: []string{"native.c"}, CXXFiles: []string{"native.cc"},
		MFiles: []string{"native.m"}, HFiles: []string{"native.h"},
		FFiles: []string{"native.f"}, SFiles: []string{"native.s"},
		SwigFiles: []string{"native.swig"}, SwigCXXFiles: []string{"native.swigcxx"},
		SysoFiles: []string{"native.syso"}, EmbedFiles: []string{"assets/config.json"},
		TestGoFiles: []string{"dep_test.go"}, XTestGoFiles: []string{"external_test.go"},
		TestEmbedFiles: []string{"testdata/unit.txt"}, XTestEmbedFiles: []string{"testdata/external.txt"},
	})

	// When the production closure is decoded.
	files, err := decodeProductionDependencyFiles(root, input)

	// Then every production class and lock input is present, while test classes are absent.
	require.NoError(t, err)
	expected := []string{
		"go.mod", "go.sum", "mise.lock", "mise.toml",
		"internal/dep/assets/config.json", "internal/dep/bridge.go", "internal/dep/dep.go",
		"internal/dep/native.c", "internal/dep/native.cc", "internal/dep/native.f",
		"internal/dep/native.h", "internal/dep/native.m", "internal/dep/native.s",
		"internal/dep/native.swig", "internal/dep/native.swigcxx", "internal/dep/native.syso",
	}
	sort.Strings(expected)
	require.Equal(t, expected, files)
}

func TestDecodeProductionDependencyFiles_rejects_hostile_embed_paths(t *testing.T) {
	tests := []struct {
		name       string
		packageDir string
		embed      string
	}{
		{name: "parent escape", packageDir: "internal/dep", embed: "../secret.txt"},
		{name: "absolute", packageDir: "internal/dep", embed: "/tmp/secret.txt"},
		{name: "unclean alias", packageDir: "internal/dep", embed: "assets/../secret.txt"},
		{name: "package outside root", packageDir: "../outside", embed: "secret.txt"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given an embed entry that is not a stable local path under the module root.
			root := t.TempDir()
			input := encodeGoListFixtures(t, goListFixturePackage{
				Dir:        filepath.Join(root, test.packageDir),
				EmbedFiles: []string{test.embed},
				Module:     &goListFixtureModule{Main: true},
			})

			// When the production closure is decoded.
			_, err := decodeProductionDependencyFiles(root, input)

			// Then the hostile path fails closed.
			require.ErrorContains(t, err, "production dependency path")
		})
	}
}

func TestDecodeProductionDependencyFiles_rejects_duplicate_file(t *testing.T) {
	// Given one source file reported in two production file classes.
	root := t.TempDir()
	input := encodeGoListFixtures(t, goListFixturePackage{
		Dir:        root,
		GoFiles:    []string{"main.go"},
		EmbedFiles: []string{"main.go"},
		Module:     &goListFixtureModule{Main: true},
	})

	// When the production closure is decoded.
	_, err := decodeProductionDependencyFiles(root, input)

	// Then the ambiguous duplicate fails closed.
	require.ErrorContains(t, err, "duplicate production build input")
}

func TestHashProductionBuildKey_rejects_missing_input(t *testing.T) {
	// Given a declared production input that is absent.
	root := t.TempDir()

	// When the build key is computed.
	_, err := hashProductionBuildKey(context.Background(), &productionBuildHashOptions{
		Root: root, Files: []string{"missing.go"}, Config: canonicalBuildConfig(), Toolchain: "go1.26.5",
	})

	// Then the missing input fails closed.
	require.ErrorContains(t, err, "inspect production build input")
}

func TestHashProductionBuildKey_rejects_duplicate_input(t *testing.T) {
	// Given the same regular input declared twice.
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/duplicate\n"), 0o600))

	// When the build key is computed.
	_, err := hashProductionBuildKey(context.Background(), &productionBuildHashOptions{
		Root: root, Files: []string{"go.mod", "go.mod"}, Config: canonicalBuildConfig(), Toolchain: "go1.26.5",
	})

	// Then the duplicate fails closed.
	require.ErrorContains(t, err, "duplicate production build input")
}

func TestHashProductionBuildKey_rejects_symlink_input(t *testing.T) {
	// Given a production input that resolves through a symbolic link.
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "target.go"), []byte("package main\n"), 0o600))
	require.NoError(t, os.Symlink("target.go", filepath.Join(root, "linked.go")))

	// When the build key is computed.
	_, err := hashProductionBuildKey(context.Background(), &productionBuildHashOptions{
		Root: root, Files: []string{"linked.go"}, Config: canonicalBuildConfig(), Toolchain: "go1.26.5",
	})

	// Then the symlink fails closed.
	require.ErrorContains(t, err, "production build input file type")
}

func TestHashProductionBuildKey_rejects_unclean_path_alias(t *testing.T) {
	// Given a path alias that normalizes to an existing production input.
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/alias\n"), 0o600))

	// When the build key is computed.
	_, err := hashProductionBuildKey(context.Background(), &productionBuildHashOptions{
		Root: root, Files: []string{"nested/../go.mod"}, Config: canonicalBuildConfig(), Toolchain: "go1.26.5",
	})

	// Then the unstable alias fails closed.
	require.ErrorContains(t, err, "production dependency path")
}

func TestReadOpenedProductionFile_rejects_replaced_path(t *testing.T) {
	// Given an opened file whose path is replaced before it is read.
	root := t.TempDir()
	path := filepath.Join(root, "input.go")
	replacement := filepath.Join(root, "replacement.go")
	require.NoError(t, os.WriteFile(path, []byte("package original\n"), 0o600))
	require.NoError(t, os.WriteFile(replacement, []byte("package replacement\n"), 0o600))
	file, err := os.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, file.Close()) })
	require.NoError(t, os.Rename(replacement, path))

	// When the opened file is read against its original path.
	_, err = readOpenedProductionFile(path, file)

	// Then the path replacement fails closed.
	require.ErrorContains(t, err, "unstable production build input")
}

func encodeGoListFixtures(t *testing.T, packages ...goListFixturePackage) *bytes.Reader {
	t.Helper()
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	for index := range packages {
		require.NoError(t, encoder.Encode(&packages[index]))
	}
	return bytes.NewReader(output.Bytes())
}
