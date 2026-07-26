package melange

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func Test_Stage_reports_empty_spec_workdir_lock_and_metadata_boundaries(t *testing.T) {
	// Given / When / Then
	paths := testPaths(t.TempDir())
	require.ErrorIs(t, Stage(&paths, Spec{}), errEmptySpec)

	writeTestFile(t, paths.LockFile, "{")
	require.ErrorContains(t, Stage(&paths, Spec{Bespoke: []string{"sentinel.yaml"}}), "parse lock file")

	writeTestFile(t, paths.LockFile, `{"packages":{},"pipeline_files":{}}`)
	require.ErrorIs(t, Stage(&paths, Spec{Upstream: "missing"}), errMissingLockMetadata)

	outside := filepath.Join(filepath.Dir(paths.Root), "outside-work")
	paths.WorkDir = outside
	require.ErrorContains(t, Stage(&paths, Spec{Bespoke: []string{"sentinel.yaml"}}), "prepare work directory")
}

func Test_stageDestination_and_stageContext_propagate_managed_write_errors(t *testing.T) {
	// Given
	rootPath := t.TempDir()
	root, err := os.OpenRoot(rootPath)
	require.NoError(t, err)
	require.NoError(t, root.Close())
	destination := stageDestination{root: root, workDir: "work"}

	// When / Then
	require.Error(t, destination.reset("specs"))
	require.ErrorContains(t, destination.write("specs/sentinel/build.yaml", []byte("recipe")), "create")

	paths := testPaths(t.TempDir())
	writeTestFile(t, filepath.Join(paths.BespokeDir, "sentinel.yaml"), "recipe")
	stage := stageContext{paths: &paths, destination: destination}
	require.Error(t, stage.bespokeRecipes([]string{"sentinel.yaml"}))

	recipe := "recipe"
	writeTestFile(t, filepath.Join(paths.LockedDir, "sentinel.yaml"), recipe)
	stage.lock = lockFile{Packages: map[string]lockPackage{
		"sentinel": {File: "sentinel.yaml", SHA256: testSHA(recipe), Assets: map[string]string{}},
	}}
	require.Error(t, stage.lockedRecipe("sentinel"))

	stage.lock = lockFile{PipelineFiles: map[string]string{}}
	require.ErrorContains(t, stage.pipelines(map[string]string{}), "reset staged pipelines")
}

func Test_stage_pipeline_and_artifact_inputs_report_verification_and_record_success_paths(t *testing.T) {
	// Given
	paths := testPaths(t.TempDir())
	writeTestFile(t, filepath.Join(paths.PipelinesDir, "sentinel.yaml"), "actual")
	root, err := os.OpenRoot(paths.Root)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, root.Close()) })
	stage := stageContext{
		paths:       &paths,
		destination: stageDestination{root: root, workDir: "melange-work"},
	}

	// When / Then
	require.ErrorIs(t, stage.pipelines(map[string]string{"sentinel.yaml": testSHA("expected")}), errChecksumMismatch)

	inputs := map[string]string{}
	require.NoError(t, addPipelineInputDigests(inputs, &paths, map[string]string{"sentinel.yaml": testSHA("actual")}))
	assert.Contains(t, inputs, "pipeline/sentinel.yaml")

	writeTestFile(t, filepath.Join(paths.BespokeDir, "sentinel.yaml"), "recipe")
	_, err = artifactInputDigests(&paths, lockFile{PipelineFiles: map[string]string{"other.yaml": testSHA("x")}}, Spec{Bespoke: []string{"sentinel.yaml"}})
	require.ErrorIs(t, err, errFileSetMismatch)
	_, err = artifactInputDigests(&paths, lockFile{PipelineFiles: map[string]string{"sentinel.yaml": testSHA("actual")}}, Spec{Bespoke: []string{"sentinel.yaml"}, EnvFile: "missing.env"})
	require.ErrorContains(t, err, "verify environment file")
}

func Test_Build_and_Prepare_report_incomplete_keys_prepare_and_keygen_errors(t *testing.T) {
	// Given
	root := t.TempDir()
	paths, spec := setupStagedUpstreamBuild(t, root)
	writeTestFile(t, filepath.Join(paths.WorkDir, "melange.rsa"), "private")

	// When / Then
	require.ErrorIs(t, Build(context.Background(), &BuildOptions{
		Paths: paths, Spec: spec, Arch: ArchitectureX8664, Staged: true, Runner: &orchestrationRunner{},
	}), errIncompleteSigningKeyPair)

	nonStaged := testPaths(t.TempDir())
	require.ErrorContains(t, Build(context.Background(), &BuildOptions{
		Paths: nonStaged, Spec: Spec{Bespoke: []string{"missing.yaml"}}, Arch: ArchitectureX8664,
	}), "read lock file")

	keygenPaths := testPaths(t.TempDir())
	generated, err := prepareBuildInputs(context.Background(), &BuildOptions{
		Paths: keygenPaths, Staged: true, Runner: &orchestrationRunner{failVerb: "keygen"},
	})
	assert.False(t, generated)
	require.ErrorIs(t, err, errSentinelBuildRunner)
}

func Test_Prepare_reports_incomplete_key_after_successful_staging(t *testing.T) {
	// Given
	root := t.TempDir()
	paths := testPaths(root)
	writeTestFile(t, paths.LockFile, `{"packages":{},"pipeline_files":{}}`)
	writeTestFile(t, filepath.Join(paths.BespokeDir, "sentinel.yaml"), "recipe")
	writeTestFile(t, filepath.Join(paths.WorkDir, "melange.rsa"), "private")

	// When
	err := Prepare(context.Background(), &BuildOptions{
		Paths: paths, Spec: Spec{Bespoke: []string{"sentinel.yaml"}}, Runner: &orchestrationRunner{},
	})

	// Then
	require.ErrorIs(t, err, errIncompleteSigningKeyPair)
}

func Test_signPackageIndexes_reports_public_key_and_compatibility_copy_errors(t *testing.T) {
	// Given
	paths := testPaths(t.TempDir())

	// When / Then
	err := signPackageIndexes(context.Background(), &BuildOptions{Paths: paths, Arch: ArchitectureX8664, Runner: &orchestrationRunner{}})
	require.ErrorContains(t, err, "copy public key")

	writeTestFile(t, filepath.Join(paths.WorkDir, "melange.rsa.pub"), "public")
	require.NoError(t, os.MkdirAll(filepath.Join(paths.RepoDir, "melange.rsa.pub"), 0o755))
	err = signPackageIndexes(context.Background(), &BuildOptions{Paths: paths, Arch: ArchitectureX8664, Runner: &orchestrationRunner{}})
	require.ErrorContains(t, err, "copy compatibility public key")
}

func Test_pinnedPackageResolver_reports_missing_arch_and_dependency_errors(t *testing.T) {
	// Given
	resolver := pinnedPackageResolver{
		architectures: []Architecture{ArchitectureX8664, ArchitectureAArch64},
		packageSets: pinnedPackageSets{
			ArchitectureX8664: {
				"application": {Name: "application", Version: "1.0-r0", Dependencies: []string{"library>=2.0-r0"}},
				"library":     {Name: "library", Version: "1.0-r0"},
			},
			ArchitectureAArch64: {
				"application": {Name: "application", Version: "1.0-r0", Dependencies: []string{"library>=2.0-r0"}},
			},
		},
	}

	// When / Then
	_, _, err := resolver.packageVersion("library")
	require.ErrorIs(t, err, errPinnedPackageMissingArch)
	_, err = resolver.localDependencies("application")
	require.ErrorIs(t, err, errPinnedDependencyConstraint)

	packages := &yaml.Node{Kind: yaml.SequenceNode}
	err = resolver.appendPinnedDependencies(packages, []string{"application"}, map[string]struct{}{"application": {}})
	require.ErrorIs(t, err, errPinnedDependencyConstraint)
}
