package buildmetadata

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComputeBuildKey_is_stable_when_only_docs_or_tests_change(t *testing.T) {
	// Given a module with production, documentation, and test-only inputs.
	root := newBuildKeyFixture(t)
	options := BuildKeyOptions{Root: root, Config: CanonicalBuildConfig()}
	before, err := ComputeBuildKey(context.Background(), options)
	require.NoError(t, err)

	// When only documentation, Go tests, and an unreferenced test helper change.
	writeBuildKeyFile(t, root, "README.md", "changed docs\n")
	writeBuildKeyFile(t, root, "main_test.go", "package main\n\nfunc Example() {}\n")
	writeBuildKeyFile(t, root, "testonly/helper.go", "package testonly\n\nconst Value = 2\n")
	after, err := ComputeBuildKey(context.Background(), options)

	// Then the production build key is unchanged.
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func TestComputeBuildKey_changes_for_every_declared_build_input(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, string, *BuildConfig)
	}{
		{name: "production Go source", mutate: func(t *testing.T, root string, _ *BuildConfig) {
			writeBuildKeyFile(t, root, "internal/value/value.go", "package value\n\nconst Number = 2\n")
		}},
		{name: "go.mod", mutate: func(t *testing.T, root string, _ *BuildConfig) {
			writeBuildKeyFile(t, root, "go.mod", "module example.com/buildkey\n\ngo 1.26.5\n\n// changed\n")
		}},
		{name: "go.sum", mutate: func(t *testing.T, root string, _ *BuildConfig) {
			writeBuildKeyFile(t, root, "go.sum", "\n")
		}},
		{name: "mise.toml", mutate: func(t *testing.T, root string, _ *BuildConfig) {
			writeBuildKeyFile(t, root, "mise.toml", "[tools]\ngo = \"1.26.6\"\n")
		}},
		{name: "GOOS", mutate: func(_ *testing.T, _ string, config *BuildConfig) { config.GOOS = "darwin" }},
		{name: "GOARCH", mutate: func(_ *testing.T, _ string, config *BuildConfig) { config.GOARCH = "arm64" }},
		{name: "CGO_ENABLED", mutate: func(_ *testing.T, _ string, config *BuildConfig) { config.CGOEnabled = "1" }},
		{name: "build flags", mutate: func(_ *testing.T, _ string, config *BuildConfig) {
			config.Flags = append(config.Flags, "-tags=changed")
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given a clean production module and canonical build configuration.
			root := newBuildKeyFixture(t)
			config := CanonicalBuildConfig()
			config.Flags = append([]string(nil), config.Flags...)
			before, err := ComputeBuildKey(context.Background(), BuildKeyOptions{Root: root, Config: config})
			require.NoError(t, err)

			// When one declared build input changes.
			test.mutate(t, root, &config)
			after, err := ComputeBuildKey(context.Background(), BuildKeyOptions{Root: root, Config: config})

			// Then the build key changes and remains a full lowercase SHA-256 digest.
			require.NoError(t, err)
			assert.NotEqual(t, before, after)
			assert.Regexp(t, `^[0-9a-f]{64}$`, after)
		})
	}
}

func TestComputeBuildKey_is_deterministic_for_identical_inputs(t *testing.T) {
	// Given one immutable module and canonical build configuration.
	root := newBuildKeyFixture(t)
	options := BuildKeyOptions{Root: root, Config: CanonicalBuildConfig()}

	// When the build key is computed twice.
	first, err := ComputeBuildKey(context.Background(), options)
	require.NoError(t, err)
	second, err := ComputeBuildKey(context.Background(), options)

	// Then both computations return the same digest.
	require.NoError(t, err)
	assert.Equal(t, first, second)
}

func newBuildKeyFixture(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	writeBuildKeyFile(t, root, "go.mod", "module example.com/buildkey\n\ngo 1.26.5\n")
	writeBuildKeyFile(t, root, "go.sum", "")
	writeBuildKeyFile(t, root, "mise.toml", "[tools]\ngo = \"1.26.5\"\n")
	writeBuildKeyFile(t, root, "README.md", "docs\n")
	writeBuildKeyFile(t, root, "main.go", `package main

import "example.com/buildkey/internal/value"

func main() { println(value.Number) }
`)
	writeBuildKeyFile(t, root, "main_test.go", "package main\n")
	writeBuildKeyFile(t, root, "internal/value/value.go", "package value\n\nconst Number = 1\n")
	writeBuildKeyFile(t, root, "internal/value/value_test.go", "package value\n")
	writeBuildKeyFile(t, root, "testonly/helper.go", "package testonly\n\nconst Value = 1\n")
	return root
}

func writeBuildKeyFile(t *testing.T, root, name, content string) {
	t.Helper()

	path := filepath.Join(root, filepath.FromSlash(name))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}
