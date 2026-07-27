package sitepublication

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/ci/publication"
)

func TestExecuteSigner_rejects_returned_output_digest_mismatch(t *testing.T) {
	// Given
	publicationPlan, _ := validPlanAndManifest(t)
	request := signerRequest(t, &publicationPlan, t.TempDir())
	key := writeSignerSentinelKeyPair(t, request)
	plan, err := BuildSignerPlan(request)
	require.NoError(t, err)
	runner := &recordingExecutionRunner{run: func(index int, _ ExecutionCommand) (ExecutionResult, error) {
		if index != 2 {
			return ExecutionResult{}, nil
		}
		writeSignerFixtureFile(t, request, filepath.Join(request.OutputAPKPath, "repository-format"))
		encoded, marshalErr := json.Marshal(&SignerOperationResult{OutputPath: signerOutputPath, OutputDigest: digestOf("f")})
		require.NoError(t, marshalErr)
		return ExecutionResult{Stdout: encoded}, nil
	}}

	// When
	result, err := ExecuteSigner(context.Background(), &plan, key, runner)

	// Then
	require.ErrorIs(t, err, ErrSignerExecution)
	assert.False(t, result.Signed)
	assert.Empty(t, result.OutputDigest)
}

func TestBuildSignerPlan_binds_plan_manifest_operations_and_package_digests(t *testing.T) {
	// Given
	publicationPlan, manifest := validPlanAndManifest(t)
	request := signerRequest(t, &publicationPlan, t.TempDir())
	packagePath := filepath.Join(request.WorkspaceDir, request.PackagesPath, "demo.apk")
	contentDigest, err := fileDigest(packagePath)
	require.NoError(t, err)
	semanticDigest, err := signerPackageSemanticDigest(packagePath)
	require.NoError(t, err)

	// When
	plan, err := BuildSignerPlan(request)

	// Then
	require.NoError(t, err)
	assert.Equal(t, publicationPlan.PlanDigest, plan.Authorization.PublicationPlanDigest)
	assert.Equal(t, publicationPlan.ManifestDigest, plan.Authorization.ManifestDigest)
	assert.Equal(t, manifest.APKOperations, plan.Authorization.APKOperations)
	require.Len(t, plan.Authorization.Packages, 1)
	assert.Equal(t, contentDigest, plan.Authorization.Packages[0].ContentDigest)
	assert.Equal(t, publication.Digest(semanticDigest), plan.Authorization.Packages[0].SemanticDigest)
	assert.Contains(t, plan.Steps[2].Command.Args, "--publication-plan-digest="+string(publicationPlan.PlanDigest))
	assert.Contains(t, plan.Steps[2].Command.Args, "--manifest-digest="+string(publicationPlan.ManifestDigest))
	assert.Contains(t, plan.Steps[2].Command.Args, "--input-digest="+string(plan.InputDigest))
}

func TestVerifySignerInputs_rejects_package_changed_after_plan(t *testing.T) {
	// Given
	publicationPlan, _ := validPlanAndManifest(t)
	request := signerRequest(t, &publicationPlan, t.TempDir())
	plan, err := BuildSignerPlan(request)
	require.NoError(t, err)
	packagePath := filepath.Join(request.WorkspaceDir, request.PackagesPath, "demo.apk")
	require.NoError(t, os.WriteFile(packagePath, []byte("tampered"), 0o644))

	// When
	err = VerifySignerInputs(request.WorkspaceDir, &plan.Authorization, plan.InputDigest, publicationPlan.PlanDigest, publicationPlan.ManifestDigest)

	// Then
	require.ErrorIs(t, err, ErrInvalidSignerPlan)
}

func writeBoundSignerFixtures(t *testing.T, request *SignerRequest) {
	t.Helper()
	plan, manifest := validPlanAndManifest(t)
	require.Equal(t, request.Plan.PlanDigest, plan.PlanDigest)
	manifestBytes, err := publication.MarshalCanonical(&manifest)
	require.NoError(t, err)
	writeSignerFixtureBytes(t, request, request.ManifestPath, manifestBytes)
	packageRelative := filepath.Join(request.PackagesPath, "demo.apk")
	writeSignerTestAPK(t, filepath.Join(request.WorkspaceDir, packageRelative), "demo", "x86_64")
	writeSignerTestAPK(t, filepath.Join(request.WorkspaceDir, request.BaseAPKPath, "base.apk"), "base", "x86_64")
	semanticDigest, err := signerPackageSemanticDigest(filepath.Join(request.WorkspaceDir, packageRelative))
	require.NoError(t, err)
	delta := map[string]any{
		"format_version":    1,
		"base_sha256":       string(digestOf("a")),
		"repository_format": "v1",
		"key_sha256":        string(digestOf("b")),
		"operations": []map[string]string{{
			"action": "upsert", "architecture": "x86_64", "package": "demo",
			"source": "demo.apk", "sha256": string(semanticDigest),
		}},
	}
	deltaBytes, err := json.Marshal(delta)
	require.NoError(t, err)
	writeSignerFixtureBytes(t, request, request.DeltaManifestPath, deltaBytes)
}

func writeSignerFixtureBytes(t *testing.T, request *SignerRequest, relative string, contents []byte) {
	t.Helper()
	path := filepath.Join(request.WorkspaceDir, filepath.FromSlash(relative))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, contents, 0o644))
}

func writeSignerTestAPK(t *testing.T, path, name, architecture string) {
	t.Helper()
	var apk bytes.Buffer
	pkgInfo := fmt.Sprintf("pkgname = %s\npkgver = 1.0-r0\narch = %s\nsize = 7\n", name, architecture)
	apk.Write(signerTestTarGzip(t, map[string]string{".PKGINFO": pkgInfo}))
	apk.Write(signerTestTarGzip(t, map[string]string{"usr/bin/" + name: "payload"}))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, apk.Bytes(), 0o644))
}

func signerTestTarGzip(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	tarWriter := tar.NewWriter(gzipWriter)
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		contents := entries[name]
		require.NoError(t, tarWriter.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(contents))}))
		_, err := tarWriter.Write([]byte(contents))
		require.NoError(t, err)
	}
	require.NoError(t, tarWriter.Close())
	require.NoError(t, gzipWriter.Close())
	return buffer.Bytes()
}
