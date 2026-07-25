package repositoryops_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ops "github.com/verity-org/verity/internal/ci/repositoryops"
)

const characterizedDigest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

var errCharacterizedGoRebuild = errors.New("copa_discover_build.sh did not complete successfully")

func TestPatchService_Run_copiesDigestPinnedImageWhenCopaReportsNoUpdates(t *testing.T) {
	// Given
	request, err := ops.NewPatchRequest(&ops.PatchRequestInput{
		Platform:        "linux/amd64",
		Source:          "localhost:5000/foo:v1.2.3@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		ImageName:       "org/name",
		StagingRegistry: "registry.example/cache",
	})
	require.NoError(t, err)
	patcher := &fakePatcher{errors: []error{ops.ErrNoPatchUpdates}}
	runner := &fakeCommandRunner{responses: []ops.CommandResult{
		{ExitCode: 1},
		{},
		{Stdout: []byte(characterizedDigest + "\n")},
	}}
	service := ops.PatchService{Patcher: patcher, Commands: runner}

	// When
	result, err := service.Run(context.Background(), &request)

	// Then
	require.NoError(t, err)
	assert.Equal(t, "registry.example/cache:org-name-v1.2.3-amd64", result.Destination)
	assert.Equal(t, characterizedDigest, result.Digest)
	assert.True(t, result.CopiedSource)
	require.Len(t, patcher.specs, 1)
	assert.Equal(t, "reports/localhost_5000_foo_v1.2.3@sha256_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb.json", patcher.specs[0].Report)
	require.Len(t, runner.calls, 3)
	assert.Equal(t, []string{"copy", "--platform", "linux/amd64", request.Source(), result.Destination}, runner.calls[1].Args)
}

func TestPatchService_Run_retriesOSOnlyWhenGoRebuildFails(t *testing.T) {
	// Given
	request, err := ops.NewPatchRequest(&ops.PatchRequestInput{
		Platform:        "linux/arm64",
		Source:          "quay.io/example/tool:v2.0.0",
		ImageName:       "tool",
		StagingRegistry: "ghcr.io/verity/cache",
	})
	require.NoError(t, err)
	patcher := &fakePatcher{errors: []error{
		errCharacterizedGoRebuild,
		nil,
	}}
	runner := &fakeCommandRunner{responses: []ops.CommandResult{
		{Stdout: []byte(characterizedDigest + "\n")},
		{Stdout: []byte(characterizedDigest + "\n")},
	}}

	// When
	result, err := (ops.PatchService{Patcher: patcher, Commands: runner}).Run(context.Background(), &request)

	// Then
	require.NoError(t, err)
	assert.True(t, result.RetriedOSOnly)
	require.Len(t, patcher.specs, 2)
	assert.Equal(t, "os,library", patcher.specs[0].PackageTypes)
	assert.Equal(t, "os", patcher.specs[1].PackageTypes)
}

func TestPatchService_Run_usesExplicitWorkflowReport(t *testing.T) {
	// Given: patch-image already downloaded the immutable pre-scan report.
	request, err := ops.NewPatchRequest(&ops.PatchRequestInput{
		Platform:        "linux/amd64",
		Source:          "docker.io/library/alpine:3.22",
		ImageName:       "alpine",
		StagingRegistry: "ghcr.io/verity/cache",
		Report:          "pre.json",
	})
	require.NoError(t, err)
	patcher := &fakePatcher{}
	runner := &fakeCommandRunner{responses: []ops.CommandResult{
		{Stdout: []byte(characterizedDigest + "\n")},
		{Stdout: []byte(characterizedDigest + "\n")},
	}}

	// When: the typed patch service runs.
	_, err = (ops.PatchService{Patcher: patcher, Commands: runner}).Run(t.Context(), &request)

	// Then: Copa consumes the exact workflow artifact instead of a shell-derived path.
	require.NoError(t, err)
	require.Len(t, patcher.specs, 1)
	assert.Equal(t, "pre.json", patcher.specs[0].Report)
}

func TestPatchService_Run_stopsWithoutRetryWhenPatchContextIsCanceled(t *testing.T) {
	// Given
	request, err := ops.NewPatchRequest(&ops.PatchRequestInput{
		Platform:        "linux/amd64",
		Source:          "quay.io/example/tool:v2.0.0",
		ImageName:       "tool",
		StagingRegistry: "ghcr.io/verity/cache",
		GoVCSURL:        "https://github.com/example/tool",
	})
	require.NoError(t, err)
	patcher := &fakePatcher{errors: []error{context.Canceled}}
	runner := &fakeCommandRunner{}

	// When
	_, err = (ops.PatchService{Patcher: patcher, Commands: runner}).Run(context.Background(), &request)

	// Then
	require.ErrorIs(t, err, context.Canceled)
	assert.Len(t, patcher.specs, 1)
	assert.Empty(t, runner.calls)
}

func TestNewPatchRequest_rejectsMaliciousImageNameBeforeExecution(t *testing.T) {
	// Given
	maliciousName := "safe\n--push=attacker.example/image"

	// When
	_, err := ops.NewPatchRequest(&ops.PatchRequestInput{
		Platform:        "linux/amd64",
		Source:          "docker.io/library/alpine:3.22",
		ImageName:       maliciousName,
		StagingRegistry: "ghcr.io/verity/cache",
	})

	// Then
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "image name")
}

func TestPatchService_Run_rejectsMisleadingSuccessfulDigestOutput(t *testing.T) {
	// Given
	request, err := ops.NewPatchRequest(&ops.PatchRequestInput{
		Platform: "linux/amd64", Source: "docker.io/library/alpine:3.22", ImageName: "alpine", StagingRegistry: "ghcr.io/verity/cache",
	})
	require.NoError(t, err)
	runner := &fakeCommandRunner{responses: []ops.CommandResult{
		{Stdout: []byte("fatal: permission denied\n")},
		{Stdout: []byte("fatal: permission denied\n")},
	}}

	// When
	_, err = (ops.PatchService{Patcher: &fakePatcher{}, Commands: runner}).Run(context.Background(), &request)

	// Then
	require.ErrorIs(t, err, ops.ErrInvalidPatchRequest)
	require.Len(t, runner.calls, 2)
	assert.Equal(t, "digest", runner.calls[0].Args[0])
}
