package melange

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_ArtifactsExist_rejects_unsupported_architecture(t *testing.T) {
	// Given
	paths := testPaths(t.TempDir())

	// When
	got := ArtifactsExist(&paths, Spec{}, Architecture("unsupported"))

	// Then
	assert.False(t, got)
}

func Test_writeArtifactMarker_reports_nonempty_existing_marker_directory(t *testing.T) {
	// Given
	paths, _, spec := newArtifactErrorFixture(t)
	marker := artifactMarkerPath(&paths, ArchitectureX8664)
	writeTestFile(t, filepath.Join(marker, "child"), "sentinel")

	// When
	err := writeArtifactMarker(&paths, spec, ArchitectureX8664)

	// Then
	require.ErrorContains(t, err, "replace artifact marker")
}

func Test_artifactFingerprint_reports_lock_file_boundaries(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, paths Paths) Spec
		want  string
	}{
		{
			name:  "missing lock",
			setup: func(*testing.T, Paths) Spec { return Spec{Upstream: "sentinel"} },
			want:  "read lock file",
		},
		{
			name: "invalid lock json",
			setup: func(t *testing.T, paths Paths) Spec {
				writeTestFile(t, paths.LockFile, "{")
				return Spec{Upstream: "sentinel"}
			},
			want: "parse lock file",
		},
		{
			name: "missing package metadata",
			setup: func(t *testing.T, paths Paths) Spec {
				writeTestFile(t, paths.LockFile, `{"packages":{},"pipeline_files":{}}`)
				return Spec{Upstream: "sentinel"}
			},
			want: "missing file or sha256 lock metadata",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given
			paths := testPaths(t.TempDir())
			spec := tt.setup(t, paths)

			// When
			_, err := artifactFingerprint(&paths, spec, ArchitectureX8664)

			// Then
			require.ErrorContains(t, err, tt.want)
		})
	}
}

func Test_addLockedInputDigests_reports_recipe_sidecar_and_asset_boundaries(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, paths Paths) lockPackage
		want  error
	}{
		{
			name:  "metadata missing",
			setup: func(*testing.T, Paths) lockPackage { return lockPackage{} },
			want:  errMissingLockMetadata,
		},
		{
			name: "recipe checksum mismatch",
			setup: func(t *testing.T, paths Paths) lockPackage {
				writeTestFile(t, filepath.Join(paths.LockedDir, "sentinel.yaml"), "recipe")
				return lockPackage{File: "sentinel.yaml", SHA256: testSHA("different"), Assets: map[string]string{}}
			},
			want: errChecksumMismatch,
		},
		{
			name: "sidecar is not a directory",
			setup: func(t *testing.T, paths Paths) lockPackage {
				writeTestFile(t, filepath.Join(paths.LockedDir, "sentinel.yaml"), "recipe")
				writeTestFile(t, filepath.Join(paths.LockedDir, "sentinel"), "not a directory")
				return lockPackage{File: "sentinel.yaml", SHA256: testSHA("recipe"), Assets: map[string]string{}}
			},
			want: errNotRealDirectory,
		},
		{
			name: "sidecar file set mismatch",
			setup: func(t *testing.T, paths Paths) lockPackage {
				writeTestFile(t, filepath.Join(paths.LockedDir, "sentinel.yaml"), "recipe")
				writeTestFile(t, filepath.Join(paths.LockedDir, "sentinel", "extra.txt"), "extra")
				return lockPackage{File: "sentinel.yaml", SHA256: testSHA("recipe"), Assets: map[string]string{}}
			},
			want: errFileSetMismatch,
		},
		{
			name: "asset checksum mismatch",
			setup: func(t *testing.T, paths Paths) lockPackage {
				writeTestFile(t, filepath.Join(paths.LockedDir, "sentinel.yaml"), "recipe")
				writeTestFile(t, filepath.Join(paths.LockedDir, "sentinel", "asset.txt"), "actual")
				return lockPackage{
					File: "sentinel.yaml", SHA256: testSHA("recipe"),
					Assets: map[string]string{"sentinel/asset.txt": testSHA("expected")},
				}
			},
			want: errChecksumMismatch,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given
			paths := testPaths(t.TempDir())
			entry := tt.setup(t, paths)
			lock := lockFile{Packages: map[string]lockPackage{"sentinel": entry}}

			// When
			err := addLockedInputDigests(map[string]string{}, &paths, lock, "sentinel")

			// Then
			require.ErrorIs(t, err, tt.want)
		})
	}
}

func Test_artifact_input_helpers_report_bespoke_pipeline_and_override_errors(t *testing.T) {
	// Given
	paths := testPaths(t.TempDir())

	// When / Then
	require.ErrorContains(t, addBespokeInputDigests(map[string]string{}, &paths, []string{"missing.yaml"}), "verify bespoke recipe")
	require.ErrorIs(t, addPipelineInputDigests(map[string]string{}, &paths, map[string]string{"missing.yaml": testSHA("x")}), errFileSetMismatch)
	writeTestFile(t, filepath.Join(paths.PipelinesDir, "sentinel.yaml"), "actual")
	require.ErrorIs(t, addPipelineInputDigests(map[string]string{}, &paths, map[string]string{"sentinel.yaml": testSHA("expected")}), errChecksumMismatch)
	require.ErrorContains(t, addOverrideInputDigest(map[string]string{}, &paths, "missing.env"), "verify environment file")

	pipelinesFile := testPaths(t.TempDir())
	writeTestFile(t, pipelinesFile.PipelinesDir, "not a directory")
	require.ErrorContains(t, addPipelineInputDigests(map[string]string{}, &pipelinesFile, nil), "list shared pipelines")
}

func Test_artifactOutputDigests_reports_directory_index_output_and_key_boundaries(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, paths Paths)
		want  error
	}{
		{
			name: "missing architecture directory",
			setup: func(t *testing.T, paths Paths) {
				require.NoError(t, os.MkdirAll(paths.RepoDir, 0o755))
			},
			want: errNoPackageIndex,
		},
		{
			name: "architecture path is a file",
			setup: func(t *testing.T, paths Paths) {
				writeTestFile(t, filepath.Join(paths.RepoDir, string(ArchitectureX8664)), "not a directory")
			},
			want: errNotRealDirectory,
		},
		{
			name: "package index missing",
			setup: func(t *testing.T, paths Paths) {
				writeTestFile(t, filepath.Join(paths.RepoDir, string(ArchitectureX8664), "sentinel.apk"), "apk")
			},
			want: errNoPackageIndex,
		},
		{
			name: "public key missing",
			setup: func(t *testing.T, paths Paths) {
				writeTestFile(t, filepath.Join(paths.RepoDir, string(ArchitectureX8664), "APKINDEX.tar.gz"), "index")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given
			paths := testPaths(t.TempDir())
			if tt.setup != nil {
				tt.setup(t, paths)
			}

			// When
			_, err := artifactOutputDigests(&paths, ArchitectureX8664)

			// Then
			if tt.want != nil {
				require.ErrorIs(t, err, tt.want)
			} else {
				require.ErrorContains(t, err, "verify public key")
			}
		})
	}
}

func newArtifactErrorFixture(t *testing.T) (Paths, lockFile, Spec) {
	t.Helper()
	paths := testPaths(t.TempDir())
	recipe := "package:\n  name: sentinel\n"
	entry := lockPackage{File: "sentinel.yaml", SHA256: testSHA(recipe), Assets: map[string]string{}}
	lock := lockFile{Packages: map[string]lockPackage{"sentinel": entry}, PipelineFiles: map[string]string{}}
	writeTestFile(t, filepath.Join(paths.LockedDir, entry.File), recipe)
	writeTestFile(t, paths.LockFile, fmt.Sprintf(`{"packages":{"sentinel":{"file":"sentinel.yaml","sha256":%q,"assets":{}}},"pipeline_files":{}}`, entry.SHA256))
	writeTestFile(t, filepath.Join(paths.RepoDir, string(ArchitectureX8664), "APKINDEX.tar.gz"), "index")
	writeTestFile(t, filepath.Join(paths.RepoDir, "melange-"+string(ArchitectureX8664)+".rsa.pub"), "public")
	return paths, lock, Spec{Upstream: "sentinel"}
}

func Test_artifactOutputDigests_rejects_invalid_tree_entries(t *testing.T) {
	// Given
	paths := testPaths(t.TempDir())
	archDir := filepath.Join(paths.RepoDir, string(ArchitectureX8664))
	require.NoError(t, os.MkdirAll(archDir, 0o755))
	require.NoError(t, os.Symlink("missing", filepath.Join(archDir, "bad-link")))

	// When
	_, err := artifactOutputDigests(&paths, ArchitectureX8664)

	// Then
	require.ErrorIs(t, err, errInvalidTreeEntry)
}
