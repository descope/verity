package ci

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAggregateIntegerBatch_rejectsMissingShard_whenDeclaredTargetsHaveNoPackages(t *testing.T) {
	// Given: a valid plan whose only build target publishes no APKs.
	plan := integerEmptyShardPlan()

	// When: no successful shard manifest is supplied.
	_, err := AggregateIntegerBatch(&plan, nil)

	// Then: target completion remains mandatory even for an empty APK shard.
	require.ErrorIs(t, err, ErrIntegerBatchIncomplete)
}

func TestAggregateIntegerBatch_acceptsCompleteShard_whenDeclaredTargetsHaveNoPackages(t *testing.T) {
	// Given: a valid empty-package shard inventory bound to its uploaded artifact.
	plan := integerEmptyShardPlan()
	inventory, err := AggregateIntegerShard(t.Context(), &IntegerShardOptions{
		Plan: &plan, Shard: "1", OutputDir: t.TempDir(),
	})
	require.NoError(t, err)
	shard, err := FinalizeIntegerShard(&inventory, IntegerArtifactRef{
		PublicationID: "integer-publication-42-3", Name: "apk-repository-integer-publication-42-3-1", Digest: testArtifactDigest("7"),
	})
	require.NoError(t, err)

	// When: the declared shard is aggregated.
	manifest, err := AggregateIntegerBatch(&plan, []IntegerShardManifest{shard})

	// Then: the exact snapshot records the completed shard and no packages.
	require.NoError(t, err)
	assert.Len(t, manifest.Shards, 1)
	assert.Empty(t, manifest.Packages)
}

func TestAggregateIntegerBatch_rejectsUndeclaredShard_whenDeclaredShardIsComplete(t *testing.T) {
	// Given: one declared empty shard plus another well-formed but undeclared shard.
	plan := integerEmptyShardPlan()
	declared := integerEmptyShardManifest("1")
	undeclared := integerEmptyShardManifest("2")

	// When: both shard manifests are supplied to the exact batch aggregate.
	_, err := AggregateIntegerBatch(&plan, []IntegerShardManifest{declared, undeclared})

	// Then: extra producer output fails closed instead of being published.
	require.ErrorIs(t, err, ErrIntegerBatchIncomplete)
}

func integerEmptyShardPlan() IntegerBatchPlan {
	return IntegerBatchPlan{
		SchemaVersion: IntegerBatchSchemaVersion,
		SourceSHA:     testSourceSHA,
		RunID:         42,
		RunAttempt:    3,
		PublicationID: "integer-publication-42-3",
		BatchID:       "42-3",
		Mode:          IntegerBatchModeSnapshot,
		Event:         IntegerBatchEventSchedule,
		Targets: []IntegerBatchTarget{{
			Name: "base", Version: "latest", Type: "default", ArtifactKey: "base-latest-default-000000000001", Shard: "1",
			ExpectedPackages: []string{}, PublishPackages: []string{},
		}},
		Packages: []IntegerPlannedPackage{},
	}
}

func integerEmptyShardManifest(shard string) IntegerShardManifest {
	return IntegerShardManifest{
		SchemaVersion: IntegerBatchSchemaVersion,
		SourceSHA:     testSourceSHA,
		RunID:         42,
		RunAttempt:    3,
		PublicationID: "integer-publication-42-3",
		BatchID:       "42-3",
		Mode:          IntegerBatchModeSnapshot,
		Event:         IntegerBatchEventSchedule,
		Shard:         shard,
		Artifact: IntegerArtifactRef{
			PublicationID: "integer-publication-42-3",
			Name:          expectedIntegerShardArtifactName("integer-publication-42-3", shard), Digest: testArtifactDigest("7"),
		},
		Packages: []IntegerPackageFile{},
	}
}
