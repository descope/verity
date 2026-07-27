package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/ci"
)

func TestCIIntegerBatchShardCommand_acceptsMissingComponentsDir_whenShardDeclaresNoPackages(t *testing.T) {
	// Given: a declared build shard with no APK-producing targets.
	root := t.TempDir()
	plan := ci.IntegerBatchPlan{
		SchemaVersion: ci.IntegerBatchSchemaVersion,
		SourceSHA:     "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		RunID:         42,
		RunAttempt:    3,
		PublicationID: "integer-publication-42-3",
		BatchID:       "42-3",
		Mode:          ci.IntegerBatchModeSnapshot,
		Event:         ci.IntegerBatchEventSchedule,
		Targets: []ci.IntegerBatchTarget{{
			Name: "base", Version: "latest", Type: "default", ArtifactKey: "base-latest-default-000000000001", Shard: "1",
			ExpectedPackages: []string{}, PublishPackages: []string{},
		}},
		Packages: []ci.IntegerPlannedPackage{},
	}
	data, err := ci.MarshalIntegerBatchPlan(&plan)
	require.NoError(t, err)
	planPath := filepath.Join(root, "plan.json")
	require.NoError(t, os.WriteFile(planPath, data, 0o600))
	inventoryPath := filepath.Join(root, "shard", "inventory.json")

	// When: aggregation runs without a downloaded component directory.
	runIntegerBatchCLI(
		t,
		"shard", "--plan", planPath, "--shard", "1",
		"--components-dir", filepath.Join(root, "missing-components"),
		"--output-dir", filepath.Join(root, "shard"), "--inventory-output", inventoryPath,
	)

	// Then: an exact empty inventory is emitted for later artifact binding.
	inventoryData, err := os.ReadFile(inventoryPath)
	require.NoError(t, err)
	inventory, err := ci.ParseIntegerShardInventory(inventoryData)
	require.NoError(t, err)
	assert.Equal(t, "1", inventory.Shard)
	assert.Empty(t, inventory.Packages)
}

func TestCIIntegerBatchAggregateCommand_acceptsMissingShardsDir_whenPlanIsEmpty(t *testing.T) {
	// Given: a valid exact plan with no selected targets or package shards.
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
		Targets:       []ci.IntegerBatchTarget{},
		Packages:      []ci.IntegerPlannedPackage{},
	}
	data, err := ci.MarshalIntegerBatchPlan(&plan)
	require.NoError(t, err)
	planPath := filepath.Join(root, "plan.json")
	require.NoError(t, os.WriteFile(planPath, data, 0o600))
	manifestPath := filepath.Join(root, "manifest.json")

	// When: final aggregation runs without a downloaded shard directory.
	runIntegerBatchCLI(
		t,
		"aggregate", "--plan", planPath,
		"--shards-dir", filepath.Join(root, "missing-shards"), "--output", manifestPath,
	)

	// Then: the canonical empty delta manifest is emitted.
	manifestData, err := os.ReadFile(manifestPath)
	require.NoError(t, err)
	manifest, err := ci.ParseIntegerBatchManifest(manifestData)
	require.NoError(t, err)
	assert.Empty(t, manifest.Shards)
	assert.Empty(t, manifest.Packages)
}
