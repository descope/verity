package apkrepository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyDelta_preserves_all_bytes_for_semantic_noop(t *testing.T) {
	// Given a signed base and a re-signed but semantically identical declared upsert.
	fixture := newDeltaFixture(t)
	source := filepath.Join(fixture.packages, "x86_64", "demo.apk")
	writeTestAPK(t, source, "demo", "1.0-r0", "x86_64", "x86_64-demo", "new-signature")
	digest, err := PackageSemanticDigest(source)
	require.NoError(t, err)
	manifest := fixture.manifest(t, []DeltaOperation{{
		Action: "upsert", Architecture: "x86_64", PackageName: "demo",
		Source: "x86_64/demo.apk", SHA256: digest,
	}})

	// When the delta is applied.
	err = ApplyDelta(context.Background(), fixture.options(t, &manifest))

	// Then the repository is byte-identical and no signing or indexing runs.
	require.NoError(t, err)
	baseDigest, digestErr := RepositoryDigest(fixture.base)
	require.NoError(t, digestErr)
	outputDigest, digestErr := RepositoryDigest(fixture.output)
	require.NoError(t, digestErr)
	assert.Equal(t, baseDigest, outputDigest)
	assert.Empty(t, fixture.runner.calls)
}

func TestApplyDelta_replaces_all_prior_versions_and_preserves_unaffected_bytes(t *testing.T) {
	// Given an x86_64 update with unrelated packages and aarch64 index bytes.
	fixture := newDeltaFixture(t)
	writeTestAPK(t, filepath.Join(fixture.base, "x86_64", "demo-0.9-r0.apk"), "demo", "0.9-r0", "x86_64", "older", "published-signature")
	source := filepath.Join(fixture.packages, "x86_64", "demo-2.0-r0.apk")
	writeTestAPK(t, source, "demo", "2.0-r0", "x86_64", "updated", "")
	digest, err := PackageSemanticDigest(source)
	require.NoError(t, err)
	keepBefore := mustReadFile(t, filepath.Join(fixture.base, "x86_64", "keep-1.0-r0.apk"))
	armIndexBefore := mustReadFile(t, filepath.Join(fixture.base, "aarch64", "APKINDEX.tar.gz"))
	manifest := fixture.manifest(t, []DeltaOperation{{
		Action: "upsert", Architecture: "x86_64", PackageName: "demo",
		Source: "x86_64/demo-2.0-r0.apk", SHA256: digest,
	}})

	// When the delta is applied.
	err = ApplyDelta(context.Background(), fixture.options(t, &manifest))

	// Then every prior demo version is replaced and unrelated bytes remain exact.
	require.NoError(t, err)
	assert.NoFileExists(t, filepath.Join(fixture.output, "x86_64", "demo-0.9-r0.apk"))
	assert.NoFileExists(t, filepath.Join(fixture.output, "x86_64", "demo-1.0-r0.apk"))
	assert.FileExists(t, filepath.Join(fixture.output, "x86_64", "demo-2.0-r0.apk"))
	assert.Equal(t, keepBefore, mustReadFile(t, filepath.Join(fixture.output, "x86_64", "keep-1.0-r0.apk")))
	assert.Equal(t, armIndexBefore, mustReadFile(t, filepath.Join(fixture.output, "aarch64", "APKINDEX.tar.gz")))
	require.Len(t, fixture.runner.calls, 2)
	assert.Equal(t, []string{"sign", "index"}, []string{fixture.runner.calls[0].args[0], fixture.runner.calls[1].args[0]})
}

func TestApplyDelta_removes_only_explicit_package_key(t *testing.T) {
	// Given an explicit removal with other packages in the same architecture.
	fixture := newDeltaFixture(t)
	manifest := fixture.manifest(t, []DeltaOperation{{
		Action: "remove", Architecture: "x86_64", PackageName: "demo",
	}})

	// When the delta is applied.
	err := ApplyDelta(context.Background(), fixture.options(t, &manifest))

	// Then all demo versions are removed while unlisted packages survive.
	require.NoError(t, err)
	assert.NoFileExists(t, filepath.Join(fixture.output, "x86_64", "demo-1.0-r0.apk"))
	assert.FileExists(t, filepath.Join(fixture.output, "x86_64", "keep-1.0-r0.apk"))
	assert.FileExists(t, filepath.Join(fixture.output, "aarch64", "demo-1.0-r0.apk"))
	require.Len(t, fixture.runner.calls, 1)
	assert.Equal(t, "index", fixture.runner.calls[0].args[0])
}

func TestApplyDelta_supports_explicit_rename_and_both_architectures(t *testing.T) {
	// Given declared removals and renamed upserts for both required architectures.
	fixture := newDeltaFixture(t)
	operations := make([]DeltaOperation, 0, 4)
	for _, architecture := range []string{"x86_64", "aarch64"} {
		relative := filepath.Join(architecture, "renamed.apk")
		source := filepath.Join(fixture.packages, relative)
		writeTestAPK(t, source, "renamed", "1.0-r0", architecture, architecture+"-renamed", "")
		digest, err := PackageSemanticDigest(source)
		require.NoError(t, err)
		operations = append(
			operations,
			DeltaOperation{Action: "remove", Architecture: architecture, PackageName: "demo"},
			DeltaOperation{Action: "upsert", Architecture: architecture, PackageName: "renamed", Source: filepath.ToSlash(relative), SHA256: digest},
		)
	}
	manifest := fixture.manifest(t, operations)

	// When the delta is applied.
	err := ApplyDelta(context.Background(), fixture.options(t, &manifest))

	// Then each architecture is reindexed and only the declared rename occurs.
	require.NoError(t, err)
	for _, architecture := range []string{"x86_64", "aarch64"} {
		assert.NoFileExists(t, filepath.Join(fixture.output, architecture, "demo-1.0-r0.apk"))
		assert.FileExists(t, filepath.Join(fixture.output, architecture, "renamed.apk"))
	}
	require.Len(t, fixture.runner.calls, 4)
}

func TestApplyDelta_is_deterministic_for_identical_base_and_manifest(t *testing.T) {
	// Given one base, one manifest, and two independent output directories.
	fixture := newDeltaFixture(t)
	source := filepath.Join(fixture.packages, "x86_64", "demo.apk")
	writeTestAPK(t, source, "demo", "2.0-r0", "x86_64", "updated", "")
	digest, err := PackageSemanticDigest(source)
	require.NoError(t, err)
	manifest := fixture.manifest(t, []DeltaOperation{{
		Action: "upsert", Architecture: "x86_64", PackageName: "demo",
		Source: "x86_64/demo.apk", SHA256: digest,
	}})
	firstOptions := fixture.options(t, &manifest)
	firstOptions.OutputDir = filepath.Join(filepath.Dir(fixture.output), "first-output")
	secondOptions := fixture.options(t, &manifest)
	secondOptions.OutputDir = filepath.Join(filepath.Dir(fixture.output), "second-output")
	secondOptions.runner = deltaRunner(t)

	// When the same transition runs twice.
	firstErr := ApplyDelta(context.Background(), firstOptions)
	secondErr := ApplyDelta(context.Background(), secondOptions)

	// Then the managed repository digest is stable.
	require.NoError(t, firstErr)
	require.NoError(t, secondErr)
	firstDigest, digestErr := RepositoryDigest(firstOptions.OutputDir)
	require.NoError(t, digestErr)
	secondDigest, digestErr := RepositoryDigest(secondOptions.OutputDir)
	require.NoError(t, digestErr)
	assert.Equal(t, firstDigest, secondDigest)
}

type deltaFixture struct {
	base       string
	packages   string
	output     string
	manifestAt string
	privatePEM []byte
	keySHA256  string
	runner     *fakeCommandRunner
}

func newDeltaFixture(t *testing.T) *deltaFixture {
	t.Helper()
	root := t.TempDir()
	fixture := &deltaFixture{
		base: filepath.Join(root, "base"), packages: filepath.Join(root, "packages"),
		output: filepath.Join(root, "output"), manifestAt: filepath.Join(root, "delta.json"),
	}
	for _, architecture := range []string{"x86_64", "aarch64"} {
		writeTestAPK(t, filepath.Join(fixture.base, architecture, "demo-1.0-r0.apk"), "demo", "1.0-r0", architecture, architecture+"-demo", "published-signature")
		writeTestAPK(t, filepath.Join(fixture.base, architecture, "keep-1.0-r0.apk"), "keep", "1.0-r0", architecture, architecture+"-keep", "published-signature")
		writeTestIndex(t, filepath.Join(fixture.base, architecture, "APKINDEX.tar.gz"), ".SIGN.RSA256.verity.rsa.pub", "base-"+architecture)
	}
	privatePEM, publicPEM := testRSAKeyPair(t)
	fixture.privatePEM = privatePEM
	writeTestFile(t, filepath.Join(fixture.base, "verity.rsa.pub"), string(publicPEM))
	writeTestFile(t, filepath.Join(fixture.base, "repository-format"), repositoryFormatVersion+"\n")
	keyHash := sha256.Sum256(publicPEM)
	fixture.keySHA256 = "sha256:" + hex.EncodeToString(keyHash[:])
	writeTestFile(t, filepath.Join(fixture.output, "index.html"), "docs")
	fixture.runner = deltaRunner(t)
	return fixture
}

func (fixture *deltaFixture) manifest(t *testing.T, operations []DeltaOperation) DeltaManifest {
	t.Helper()
	baseDigest, err := RepositoryDigest(fixture.base)
	require.NoError(t, err)
	return DeltaManifest{
		FormatVersion: 1, BaseSHA256: baseDigest, RepositoryFormat: repositoryFormatVersion,
		KeySHA256: fixture.keySHA256, Operations: operations,
	}
}

func (fixture *deltaFixture) options(t *testing.T, manifest *DeltaManifest) *DeltaOptions {
	t.Helper()
	data, err := json.Marshal(manifest)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(fixture.manifestAt, data, 0o644))
	return &DeltaOptions{
		BaseDir: fixture.base, PackageDir: fixture.packages, ManifestPath: fixture.manifestAt,
		OutputDir: fixture.output, PrivateKeyPEM: fixture.privatePEM, runner: fixture.runner,
	}
}

func deltaRunner(t *testing.T) *fakeCommandRunner {
	t.Helper()
	return &fakeCommandRunner{run: func(request command) (commandResult, error) {
		if request.name != "melange" || len(request.args) == 0 {
			return commandResult{}, fmt.Errorf("%w: %s", errUnexpectedCommand, request.name)
		}
		switch request.args[0] {
		case "sign":
			return commandResult{}, nil
		case "index":
			packages := make([]string, 0)
			for _, arg := range request.args {
				if filepath.Ext(arg) == ".apk" {
					packages = append(packages, filepath.Base(arg))
				}
			}
			slices.Sort(packages)
			writeTestIndex(t, filepath.Join(request.dir, "APKINDEX.tar.gz"), append([]string{".SIGN.RSA256.verity.rsa.pub"}, packages...)...)
			return commandResult{}, nil
		default:
			return commandResult{}, fmt.Errorf("%w: melange %s", errUnexpectedCommand, request.args[0])
		}
	}}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	contents, err := os.ReadFile(path)
	require.NoError(t, err)
	return contents
}
