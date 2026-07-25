package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComputeProductionBuildKey_hashes_only_imported_module_closure(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		replacement string
		wantChange  bool
	}{
		{name: "root test", path: "main_test.go", replacement: "package main\n\nfunc changedForTest() {}\n"},
		{name: "test tree", path: "test/harness.go", replacement: "package test\n\nconst Harness = \"changed\"\n"},
		{name: "documentation", path: "README.md", replacement: "changed documentation\n"},
		{name: "unimported package", path: "unused/unused.go", replacement: "package unused\n\nconst Value = \"changed\"\n"},
		{name: "go.mod", path: "go.mod", replacement: "module example.com/buildkey\n\ngo 1.23\n\n// changed\n", wantChange: true},
		{name: "go.sum", path: "go.sum", replacement: "\n", wantChange: true},
		{name: "mise toolchain", path: "mise.toml", replacement: "[tools]\ngo = \"1.24\"\n", wantChange: true},
		{name: "mise toolchain lock", path: "mise.lock", replacement: "[tools.go]\nversion = \"1.24.1\"\nchecksum = \"changed\"\n", wantChange: true},
		{name: "imported package", path: "internal/dep/dep.go", replacement: "package dep\n\nimport _ \"embed\"\n\n//go:embed data.txt\nvar Value string\n\nconst Changed = true\n", wantChange: true},
		{name: "embedded production file", path: "internal/dep/data.txt", replacement: "changed production data\n", wantChange: true},
		{name: "linux amd64 assembly", path: "internal/dep/value_amd64.s", replacement: "#include \"textflag.h\"\n\nTEXT ·NativeValue(SB), NOSPLIT, $0-8\n\tMOVQ $2, ret+0(FP)\n\tRET\n", wantChange: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given a module with imported production code, embedded data, tests, and an unimported package.
			root := writeBuildKeyModule(t)
			baseline, err := computeProductionBuildKey(context.Background(), root, canonicalBuildConfig())
			require.NoError(t, err)

			// When one source-tree input changes.
			require.NoError(t, os.WriteFile(filepath.Join(root, test.path), []byte(test.replacement), 0o600))
			changed, err := computeProductionBuildKey(context.Background(), root, canonicalBuildConfig())

			// Then only files in the actual production dependency closure rotate the key.
			require.NoError(t, err)
			if test.wantChange {
				assert.NotEqual(t, baseline, changed)
			} else {
				assert.Equal(t, baseline, changed)
			}
		})
	}
}

func TestHashProductionBuildKey_includes_toolchain_platform_and_flags(t *testing.T) {
	// Given one fixed production dependency closure.
	root := writeBuildKeyModule(t)
	files, err := productionDependencyFiles(context.Background(), root)
	require.NoError(t, err)
	config := canonicalBuildConfig()
	baseline, err := hashProductionBuildKey(context.Background(), &productionBuildHashOptions{
		Root: root, Files: files, Config: config, Toolchain: "go1.26.5",
	})
	require.NoError(t, err)

	// When each non-file build input changes independently.
	toolchain, err := hashProductionBuildKey(context.Background(), &productionBuildHashOptions{
		Root: root, Files: files, Config: config, Toolchain: "go1.26.6",
	})
	require.NoError(t, err)
	platformConfig := config
	platformConfig.GOARCH = "arm64"
	platform, err := hashProductionBuildKey(context.Background(), &productionBuildHashOptions{
		Root: root, Files: files, Config: platformConfig, Toolchain: "go1.26.5",
	})
	require.NoError(t, err)
	flagsConfig := config
	flagsConfig.Flags = append(flagsConfig.Flags, "-tags=production")
	flags, err := hashProductionBuildKey(context.Background(), &productionBuildHashOptions{
		Root: root, Files: files, Config: flagsConfig, Toolchain: "go1.26.5",
	})
	require.NoError(t, err)

	// Then toolchain, platform, and compiler flags all rotate the key.
	assert.NotEqual(t, baseline, toolchain)
	assert.NotEqual(t, baseline, platform)
	assert.NotEqual(t, baseline, flags)
}

func TestHashProductionBuildKey_preserves_exact_flag_order(t *testing.T) {
	// Given one fixed production dependency closure and reordered safe build flags.
	root := writeBuildKeyModule(t)
	files, err := productionDependencyFiles(context.Background(), root)
	require.NoError(t, err)
	canonical := canonicalBuildConfig()
	reordered := canonicalBuildConfig()
	reordered.Flags[0], reordered.Flags[1] = reordered.Flags[1], reordered.Flags[0]

	// When each exact invocation is hashed.
	canonicalKey, err := hashProductionBuildKey(context.Background(), &productionBuildHashOptions{
		Root: root, Files: files, Config: canonical, Toolchain: "go1.26.5",
	})
	require.NoError(t, err)
	reorderedKey, err := hashProductionBuildKey(context.Background(), &productionBuildHashOptions{
		Root: root, Files: files, Config: reordered, Toolchain: "go1.26.5",
	})

	// Then changing flag order changes the build key.
	require.NoError(t, err)
	assert.NotEqual(t, canonicalKey, reorderedKey)
}

func writeBuildKeyModule(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"go.mod":                     "module example.com/buildkey\n\ngo 1.23\n",
		"go.sum":                     "",
		"mise.toml":                  "[tools]\ngo = \"1.23\"\n",
		"mise.lock":                  "[tools.go]\nversion = \"1.23.0\"\nchecksum = \"baseline\"\n",
		"main.go":                    "package main\n\nimport \"example.com/buildkey/internal/dep\"\n\nfunc main() { _, _ = dep.Value, dep.NativeValue() }\n",
		"main_test.go":               "package main\n\nfunc unchangedForTest() {}\n",
		"README.md":                  "documentation\n",
		"internal/dep/dep.go":        "package dep\n\nimport _ \"embed\"\n\n//go:embed data.txt\nvar Value string\n\nfunc NativeValue() uint64\n",
		"internal/dep/data.txt":      "production data\n",
		"internal/dep/value_amd64.s": "#include \"textflag.h\"\n\nTEXT ·NativeValue(SB), NOSPLIT, $0-8\n\tMOVQ $1, ret+0(FP)\n\tRET\n",
		"test/harness.go":            "package test\n\nconst Harness = \"test-only\"\n",
		"unused/unused.go":           "package unused\n\nconst Value = \"unimported\"\n",
	}
	for name, data := range files {
		path := filepath.Join(root, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte(data), 0o600))
	}
	return root
}
