package ci

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const integerJSONSourceSHA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func integerJSONDigest(seed string) string {
	return "sha256:" + strings.Repeat(seed, 64)
}

func integerJSONPlanFixture() IntegerBatchPlan {
	return IntegerBatchPlan{
		SchemaVersion: IntegerBatchSchemaVersion,
		SourceSHA:     integerJSONSourceSHA,
		RunID:         42,
		RunAttempt:    3,
		PublicationID: "integer-json-42-3",
		BatchID:       "42-3",
		Mode:          IntegerBatchModeSnapshot,
		Event:         IntegerBatchEventSchedule,
		Targets: []IntegerBatchTarget{
			{Name: "beta", Version: "1", Type: "default", ArtifactKey: "beta-1-default-000000000002", Shard: "2", ExpectedPackages: []string{"beta"}, PublishPackages: []string{"beta"}},
			{Name: "alpha", Version: "1", Type: "default", ArtifactKey: "alpha-1-default-000000000001", Shard: "1", ExpectedPackages: []string{"alpha"}, PublishPackages: []string{"alpha"}},
		},
		Packages: []IntegerPlannedPackage{
			{Architecture: IntegerArchitectureX8664, Name: "beta", Producer: "beta:1-default"},
			{Architecture: IntegerArchitectureAArch64, Name: "beta", Producer: "beta:1-default"},
			{Architecture: IntegerArchitectureX8664, Name: "alpha", Producer: "alpha:1-default"},
			{Architecture: IntegerArchitectureAArch64, Name: "alpha", Producer: "alpha:1-default"},
		},
	}
}

func integerJSONPackageFiles() []IntegerPackageFile {
	return []IntegerPackageFile{
		{Architecture: IntegerArchitectureX8664, Name: "zeta", SHA256: integerJSONDigest("1"), Path: "x86_64/zeta.apk"},
		{Architecture: IntegerArchitectureAArch64, Name: "zeta", SHA256: integerJSONDigest("2"), Path: "aarch64/zeta.apk"},
		{Architecture: IntegerArchitectureAArch64, Name: "alpha", SHA256: integerJSONDigest("3"), Path: "aarch64/alpha.apk"},
	}
}

func integerJSONComponentFixture() IntegerComponentManifest {
	return IntegerComponentManifest{
		SchemaVersion: IntegerBatchSchemaVersion,
		SourceSHA:     integerJSONSourceSHA,
		RunID:         42,
		RunAttempt:    3,
		PublicationID: "integer-json-42-3",
		BatchID:       "42-3",
		Mode:          IntegerBatchModeSnapshot,
		Event:         IntegerBatchEventSchedule,
		TargetID:      "alpha:1-default",
		Shard:         "1",
		Packages:      integerJSONPackageFiles(),
	}
}

func integerJSONInventoryFixture() IntegerShardInventory {
	component := integerJSONComponentFixture()
	return IntegerShardInventory{
		SchemaVersion: component.SchemaVersion,
		SourceSHA:     component.SourceSHA,
		RunID:         component.RunID,
		RunAttempt:    component.RunAttempt,
		PublicationID: component.PublicationID,
		BatchID:       component.BatchID,
		Mode:          component.Mode,
		Event:         component.Event,
		Shard:         component.Shard,
		Packages:      integerJSONPackageFiles(),
	}
}

func integerJSONShardFixture(shard string) IntegerShardManifest {
	inventory := integerJSONInventoryFixture()
	inventory.Shard = shard
	return IntegerShardManifest{
		SchemaVersion: inventory.SchemaVersion,
		SourceSHA:     inventory.SourceSHA,
		RunID:         inventory.RunID,
		RunAttempt:    inventory.RunAttempt,
		PublicationID: inventory.PublicationID,
		BatchID:       inventory.BatchID,
		Mode:          inventory.Mode,
		Event:         inventory.Event,
		Shard:         shard,
		Artifact: IntegerArtifactRef{
			PublicationID: inventory.PublicationID,
			Name:          expectedIntegerShardArtifactName(inventory.PublicationID, shard),
			Digest:        integerJSONDigest(shard),
		},
		Packages: integerJSONPackageFiles(),
	}
}

func integerJSONBatchFixture() IntegerBatchManifest {
	shardTwo := integerJSONShardFixture("2")
	shardOne := integerJSONShardFixture("1")
	return IntegerBatchManifest{
		SchemaVersion: IntegerBatchSchemaVersion,
		SourceSHA:     integerJSONSourceSHA,
		RunID:         42,
		RunAttempt:    3,
		PublicationID: "integer-json-42-3",
		BatchID:       "42-3",
		Mode:          IntegerBatchModeSnapshot,
		Event:         IntegerBatchEventSchedule,
		Shards:        []IntegerShardManifest{shardTwo, shardOne},
		Packages: []IntegerPublishedPackage{
			{Architecture: IntegerArchitectureX8664, Name: "zeta", SHA256: integerJSONDigest("4"), Artifact: shardTwo.Artifact},
			{Architecture: IntegerArchitectureAArch64, Name: "zeta", SHA256: integerJSONDigest("5"), Artifact: shardOne.Artifact},
			{Architecture: IntegerArchitectureAArch64, Name: "alpha", SHA256: integerJSONDigest("6"), Artifact: shardOne.Artifact},
		},
	}
}

func integerJSONFileKeys(packages []IntegerPackageFile) []string {
	keys := make([]string, 0, len(packages))
	for _, pkg := range packages {
		keys = append(keys, fmt.Sprintf("%s/%s", pkg.Architecture, pkg.Name))
	}
	return keys
}

func TestIntegerBatchPlanJSON_roundtrips_canonically_without_mutating_input(t *testing.T) {
	// Given
	plan := integerJSONPlanFixture()

	// When
	first, err := MarshalIntegerBatchPlan(&plan)
	require.NoError(t, err)
	second, err := MarshalIntegerBatchPlan(&plan)
	require.NoError(t, err)
	parsed, err := ParseIntegerBatchPlan(first)

	// Then
	require.NoError(t, err)
	require.Equal(t, first, second)
	require.Equal(t, "beta:1-default", plan.Targets[0].ID())
	require.Equal(t, "alpha:1-default", parsed.Targets[0].ID())
	require.Equal(t, []string{"aarch64/alpha", "aarch64/beta", "x86_64/alpha", "x86_64/beta"}, []string{
		string(parsed.Packages[0].Architecture) + "/" + parsed.Packages[0].Name,
		string(parsed.Packages[1].Architecture) + "/" + parsed.Packages[1].Name,
		string(parsed.Packages[2].Architecture) + "/" + parsed.Packages[2].Name,
		string(parsed.Packages[3].Architecture) + "/" + parsed.Packages[3].Name,
	})
}

func TestIntegerManifestJSON_roundtrips_each_boundary_canonically(t *testing.T) {
	t.Run("component manifest", func(t *testing.T) {
		// Given
		manifest := integerJSONComponentFixture()

		// When
		data, err := MarshalIntegerComponentManifest(&manifest)
		require.NoError(t, err)
		parsed, err := ParseIntegerComponentManifest(data)

		// Then
		require.NoError(t, err)
		require.Equal(t, []string{"aarch64/alpha", "aarch64/zeta", "x86_64/zeta"}, integerJSONFileKeys(parsed.Packages))
		require.Equal(t, "x86_64/zeta", integerJSONFileKeys(manifest.Packages)[0])
	})

	t.Run("shard inventory", func(t *testing.T) {
		// Given
		inventory := integerJSONInventoryFixture()

		// When
		data, err := MarshalIntegerShardInventory(&inventory)
		require.NoError(t, err)
		parsed, err := ParseIntegerShardInventory(data)

		// Then
		require.NoError(t, err)
		require.Equal(t, []string{"aarch64/alpha", "aarch64/zeta", "x86_64/zeta"}, integerJSONFileKeys(parsed.Packages))
	})

	t.Run("shard manifest", func(t *testing.T) {
		// Given
		manifest := integerJSONShardFixture("1")

		// When
		data, err := MarshalIntegerShardManifest(&manifest)
		require.NoError(t, err)
		parsed, err := ParseIntegerShardManifest(data)

		// Then
		require.NoError(t, err)
		require.Equal(t, []string{"aarch64/alpha", "aarch64/zeta", "x86_64/zeta"}, integerJSONFileKeys(parsed.Packages))
		require.Equal(t, expectedIntegerShardArtifactName(parsed.PublicationID, parsed.Shard), parsed.Artifact.Name)
	})

	t.Run("batch manifest", func(t *testing.T) {
		// Given
		manifest := integerJSONBatchFixture()

		// When
		first, err := MarshalIntegerBatchManifest(&manifest)
		require.NoError(t, err)
		parsed, err := ParseIntegerBatchManifest(first)
		require.NoError(t, err)
		second, err := MarshalIntegerBatchManifest(&parsed)

		// Then
		require.NoError(t, err)
		require.Equal(t, first, second)
		require.Equal(t, []string{"1", "2"}, []string{parsed.Shards[0].Shard, parsed.Shards[1].Shard})
		require.Equal(t, []string{"aarch64/alpha", "aarch64/zeta", "x86_64/zeta"}, []string{
			string(parsed.Packages[0].Architecture) + "/" + parsed.Packages[0].Name,
			string(parsed.Packages[1].Architecture) + "/" + parsed.Packages[1].Name,
			string(parsed.Packages[2].Architecture) + "/" + parsed.Packages[2].Name,
		})
	})
}
