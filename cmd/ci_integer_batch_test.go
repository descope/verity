package cmd

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"

	"github.com/verity-org/verity/internal/ci"
)

func TestCIIntegerBatchPlanCommand_writesExactPlanExpectedSetAndShardOutputs(t *testing.T) {
	// Given: one offline Integer image and an exact scheduled producer identity.
	root := t.TempDir()
	writeCommandFixture(t, filepath.Join(root, "integer.yaml"), "target:\n  registry: ghcr.io/test\n")
	writeCommandFixture(t, filepath.Join(root, "images", "_base", "wolfi-base.yaml"), "# base\n")
	writeCommandFixture(t, filepath.Join(root, "images", "alpha.yaml"), `
name: alpha
description: alpha
upstream:
  package: alpha
types:
  default:
    base: wolfi-base
    packages: ["alpha"]
versions:
  latest: {}
`)
	planPath := filepath.Join(root, "plan.json")
	expectedPath := filepath.Join(root, "expected.json")
	githubOutput := filepath.Join(root, "github-output")

	// When: the public production planner runs without network discovery.
	runIntegerBatchCLI(
		t,
		"plan", "--event", "schedule", "--source-sha", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"--run-id", "42", "--run-attempt", "3", "--publication-id", "integer-publication-42-3", "--batch-id", "42-3",
		"--repo-root", root, "--integer-config", filepath.Join(root, "integer.yaml"),
		"--images-dir", filepath.Join(root, "images"), "--apkindex-url", "",
		"--plan-output", planPath, "--expected-output", expectedPath, "--github-output", githubOutput,
	)

	// Then: the canonical plan, strict child expectation set, and compact shard
	// outputs all describe the same target.
	planData, err := os.ReadFile(planPath)
	require.NoError(t, err)
	plan, err := ci.ParseIntegerBatchPlan(planData)
	require.NoError(t, err)
	assert.Equal(t, ci.IntegerBatchModeSnapshot, plan.Mode)
	require.Len(t, plan.Targets, 1)
	assert.Equal(t, "alpha:latest-default", plan.Targets[0].ID())
	expected, err := os.ReadFile(expectedPath)
	require.NoError(t, err)
	assert.JSONEq(t, `[{"name":"alpha","version":"latest","type":"default"}]`, string(expected))
	outputs, err := os.ReadFile(githubOutput)
	require.NoError(t, err)
	assert.Contains(t, string(outputs), "count=1")
	assert.Contains(t, string(outputs), "shard_count=1")
	assert.Contains(t, string(outputs), "event=schedule")
	assert.Contains(t, string(outputs), "publication_id=integer-publication-42-3")
	assert.Contains(t, string(outputs), `"component_count":0`)
	assert.Contains(t, string(outputs), `"entries":"[{\"name\":\"alpha\"`)
}

func TestCIIntegerBatchOutputsCommand_readsValidatedPlanAndWritesExactMetadata(t *testing.T) {
	// Given: a canonical plan already written by the Go planner.
	root := t.TempDir()
	plan := ci.IntegerBatchPlan{
		SchemaVersion: ci.IntegerBatchSchemaVersion, SourceSHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		RunID: 42, RunAttempt: 3, PublicationID: "integer-publication-42-3", BatchID: "42-3",
		Mode: ci.IntegerBatchModeSnapshot, Event: ci.IntegerBatchEventSchedule,
		Targets: []ci.IntegerBatchTarget{{
			Name: "alpha", Version: "1", Type: "default", ArtifactKey: "alpha-1-default-000000000001", Shard: "1",
			ExpectedPackages: []string{"alpha"}, PublishPackages: []string{"alpha"},
		}},
		Packages: []ci.IntegerPlannedPackage{
			{Architecture: ci.IntegerArchitectureAArch64, Name: "alpha", Producer: "alpha:1-default"},
			{Architecture: ci.IntegerArchitectureX8664, Name: "alpha", Producer: "alpha:1-default"},
		},
	}
	data, err := ci.MarshalIntegerBatchPlan(&plan)
	require.NoError(t, err)
	planPath := filepath.Join(root, "plan.json")
	require.NoError(t, os.WriteFile(planPath, data, 0o600))
	githubOutput := filepath.Join(root, "github-output")

	// When: the dedicated output command exposes metadata from that plan.
	runIntegerBatchCLI(t, "outputs", "--plan", planPath, "--github-output", githubOutput)

	// Then: every identity and shard value comes from the validated plan.
	outputs, err := os.ReadFile(githubOutput)
	require.NoError(t, err)
	assert.Contains(t, string(outputs), "source_sha=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	assert.Contains(t, string(outputs), "publication_id=integer-publication-42-3")
	assert.Contains(t, string(outputs), "batch_id=42-3")
	assert.Contains(t, string(outputs), "count=1")
}

func TestIntegerPlanShards_capsPublicationMatricesAt64Targets(t *testing.T) {
	// Given
	targets := make([]ci.IntegerBatchTarget, 0, 65)
	for index := range 65 {
		targets = append(targets, ci.IntegerBatchTarget{
			Name: "image-" + strconv.Itoa(index), Version: "1", Type: "default",
			Shard: strconv.Itoa(ci.IntegerMatrixShard(index)),
		})
	}

	// When
	shards, err := integerPlanShards(targets)

	// Then
	require.NoError(t, err)
	require.Len(t, shards, 2)
	assert.Equal(t, []int{64, 1}, []int{shards[0].Count, shards[1].Count})
}

func TestCIIntegerBatchCommand_runsMockedExactBatchArtifactFlow(t *testing.T) {
	// Given: one exact planned package and two real architecture APK fixtures.
	root := t.TempDir()
	plan := ci.IntegerBatchPlan{
		SchemaVersion: ci.IntegerBatchSchemaVersion,
		SourceSHA:     "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		RunID:         42,
		RunAttempt:    3,
		PublicationID: "integer-publication-42-3",
		BatchID:       "42-3",
		Mode:          ci.IntegerBatchModeDelta,
		Event:         ci.IntegerBatchEventPush,
		Targets: []ci.IntegerBatchTarget{{
			Name: "alpha", Version: "1", Type: "default", ArtifactKey: "alpha-1-default-000000000001", Shard: "1",
			ExpectedPackages: []string{"alpha"}, PublishPackages: []string{"alpha"},
		}},
		Packages: []ci.IntegerPlannedPackage{
			{Architecture: ci.IntegerArchitectureX8664, Name: "alpha", Producer: "alpha:1-default"},
			{Architecture: ci.IntegerArchitectureAArch64, Name: "alpha", Producer: "alpha:1-default"},
		},
	}
	planData, err := ci.MarshalIntegerBatchPlan(&plan)
	require.NoError(t, err)
	planPath := filepath.Join(root, "plan.json")
	require.NoError(t, os.WriteFile(planPath, planData, 0o600))
	packages := filepath.Join(root, "packages")
	writeCommandTestAPK(t, filepath.Join(packages, "x86_64", "alpha.apk"), "alpha", "x86_64")
	writeCommandTestAPK(t, filepath.Join(packages, "aarch64", "alpha.apk"), "alpha", "aarch64")
	component := filepath.Join(root, "components", "alpha")
	shardPackages := filepath.Join(root, "shard-packages")
	inventoryPath := filepath.Join(root, "shard-inventory.json")
	shardPath := filepath.Join(root, "shards", "one", "shard-manifest.json")
	manifestPath := filepath.Join(root, "integer-manifest.json")

	// When: the public CLI stages, aggregates, finalizes, and publishes the
	// mocked exact batch contract.
	runIntegerBatchCLI(t, "component", "--plan", planPath, "--target", "alpha:1-default", "--packages-dir", packages, "--output-dir", component)
	runIntegerBatchCLI(t, "shard", "--plan", planPath, "--shard", "1", "--components-dir", filepath.Join(root, "components"), "--output-dir", shardPackages, "--inventory-output", inventoryPath)
	runIntegerBatchCLI(t, "finalize-shard", "--inventory", inventoryPath, "--publication-id", "integer-publication-42-3", "--artifact-name", "apk-repository-integer-publication-42-3-1", "--artifact-digest", "sha256:"+string(bytes.Repeat([]byte{'1'}, 64)), "--output", shardPath)
	runIntegerBatchCLI(t, "aggregate", "--plan", planPath, "--shards-dir", filepath.Join(root, "shards"), "--output", manifestPath)

	// Then: the final artifact is non-empty and retains exact provenance,
	// package architecture, artifact name, and digest.
	data, err := os.ReadFile(manifestPath)
	require.NoError(t, err)
	require.NotEmpty(t, data)
	manifest, err := ci.ParseIntegerBatchManifest(data)
	require.NoError(t, err)
	assert.Equal(t, "42-3", manifest.BatchID)
	require.Len(t, manifest.Packages, 2)
	assert.Equal(t, "apk-repository-integer-publication-42-3-1", manifest.Packages[0].Artifact.Name)
	assert.Equal(t, "integer-publication-42-3", manifest.PublicationID)
	assert.Regexp(t, `^sha256:[0-9a-f]{64}$`, manifest.Packages[0].SHA256)
}

func TestCIIntegerBatchCommand_rejectsMalformedPlan_beforeMutation(t *testing.T) {
	// Given: an ambiguous duplicate-key plan and a pre-existing output marker.
	root := t.TempDir()
	planPath := filepath.Join(root, "plan.json")
	require.NoError(t, os.WriteFile(planPath, []byte(`{"schema_version":1,"schema_version":2}`), 0o600))
	output := filepath.Join(root, "output")
	require.NoError(t, os.MkdirAll(output, 0o755))
	marker := filepath.Join(output, "keep")
	require.NoError(t, os.WriteFile(marker, []byte("unchanged"), 0o600))

	// When: component staging parses the hostile plan.
	err := runIntegerBatchCLIErr("component", "--plan", planPath, "--target", "alpha:1-default", "--packages-dir", root, "--output-dir", output)

	// Then: parsing fails closed before stale output is removed.
	require.ErrorIs(t, err, ci.ErrIntegerBatchPlan)
	assert.FileExists(t, marker)
}

func runIntegerBatchCLI(t *testing.T, arguments ...string) {
	t.Helper()
	require.NoError(t, runIntegerBatchCLIErr(arguments...))
}

func runIntegerBatchCLIErr(arguments ...string) error {
	command := &cli.Command{Commands: []*cli.Command{CICommand}}
	return command.Run(context.Background(), append([]string{"verity", "ci", "integer-batch"}, arguments...))
}

func writeCommandTestAPK(t *testing.T, path, name, architecture string) {
	t.Helper()
	var apk bytes.Buffer
	pkgInfo := fmt.Sprintf("pkgname = %s\npkgver = 1.0.0-r0\narch = %s\nsize = 1\n", name, architecture)
	apk.Write(commandTestTarGzip(t, map[string]string{".PKGINFO": pkgInfo}))
	apk.Write(commandTestTarGzip(t, map[string]string{"usr/bin/" + name: "x"}))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, apk.Bytes(), 0o600))
}

func commandTestTarGzip(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	tarWriter := tar.NewWriter(gzipWriter)
	for name, contents := range entries {
		require.NoError(t, tarWriter.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(contents))}))
		_, err := tarWriter.Write([]byte(contents))
		require.NoError(t, err)
	}
	require.NoError(t, tarWriter.Close())
	require.NoError(t, gzipWriter.Close())
	return buffer.Bytes()
}

func writeCommandFixture(t *testing.T, path, contents string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(strings.TrimSpace(contents)+"\n"), 0o600))
}
