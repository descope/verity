package ci

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIntegerBatchPlanJSON_rejects_each_invalid_identity_and_entry_class(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*IntegerBatchPlan)
		wantErr error
	}{
		{name: "schema version", mutate: func(plan *IntegerBatchPlan) { plan.SchemaVersion = 0 }, wantErr: ErrIntegerBatchPlan},
		{name: "source SHA", mutate: func(plan *IntegerBatchPlan) { plan.SourceSHA = "invalid" }, wantErr: ErrIntegerBatchPlan},
		{name: "run identity", mutate: func(plan *IntegerBatchPlan) { plan.RunID = 0 }, wantErr: ErrIntegerBatchPlan},
		{name: "publication ID", mutate: func(plan *IntegerBatchPlan) { plan.PublicationID = "bad publication" }, wantErr: ErrIntegerBatchPlan},
		{name: "batch identity", mutate: func(plan *IntegerBatchPlan) { plan.BatchID = "42-4" }, wantErr: ErrIntegerBatchPlan},
		{name: "aliased publication", mutate: func(plan *IntegerBatchPlan) { plan.PublicationID = plan.BatchID }, wantErr: ErrIntegerBatchPlan},
		{name: "mode", mutate: func(plan *IntegerBatchPlan) { plan.Mode = "invalid" }, wantErr: ErrIntegerBatchPlan},
		{name: "event", mutate: func(plan *IntegerBatchPlan) { plan.Event = "invalid" }, wantErr: ErrIntegerBatchPlan},
		{name: "incomplete target", mutate: func(plan *IntegerBatchPlan) { plan.Targets[0].ArtifactKey = "invalid" }, wantErr: ErrIntegerBatchPlan},
		{name: "duplicate target", mutate: func(plan *IntegerBatchPlan) { plan.Targets = append(plan.Targets, plan.Targets[0]) }, wantErr: ErrIntegerPackageDuplicate},
		{name: "duplicate artifact key", mutate: func(plan *IntegerBatchPlan) { plan.Targets[1].ArtifactKey = plan.Targets[0].ArtifactKey }, wantErr: ErrIntegerPackageDuplicate},
		{name: "duplicate expected package", mutate: func(plan *IntegerBatchPlan) {
			plan.Targets[0].ExpectedPackages = append(plan.Targets[0].ExpectedPackages, plan.Targets[0].ExpectedPackages[0])
		}, wantErr: ErrIntegerBatchPlan},
		{name: "duplicate published package", mutate: func(plan *IntegerBatchPlan) {
			plan.Targets[0].PublishPackages = append(plan.Targets[0].PublishPackages, plan.Targets[0].PublishPackages[0])
		}, wantErr: ErrIntegerBatchPlan},
		{name: "undeclared published package", mutate: func(plan *IntegerBatchPlan) {
			plan.Targets[0].PublishPackages = []string{"ghost"}
		}, wantErr: ErrIntegerBatchPlan},
		{name: "invalid package", mutate: func(plan *IntegerBatchPlan) { plan.Packages[0].Architecture = "sparc" }, wantErr: ErrIntegerBatchPlan},
		{name: "unknown producer", mutate: func(plan *IntegerBatchPlan) { plan.Packages[0].Producer = "ghost:1-default" }, wantErr: ErrIntegerBatchPlan},
		{name: "duplicate package", mutate: func(plan *IntegerBatchPlan) { plan.Packages = append(plan.Packages, plan.Packages[0]) }, wantErr: ErrIntegerPackageDuplicate},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given
			plan := integerJSONPlanFixture()
			test.mutate(&plan)

			// When
			_, err := MarshalIntegerBatchPlan(&plan)

			// Then
			require.ErrorIs(t, err, test.wantErr)
		})
	}
}

func TestIntegerManifestJSON_rejects_nil_shape_identity_and_artifact_mismatches(t *testing.T) {
	tests := []struct {
		name    string
		run     func() error
		wantErr error
	}{
		{name: "nil plan", run: func() error { _, err := MarshalIntegerBatchPlan(nil); return err }, wantErr: ErrIntegerBatchPlan},
		{name: "nil component", run: func() error { _, err := MarshalIntegerComponentManifest(nil); return err }, wantErr: ErrIntegerBatchPlan},
		{name: "nil inventory", run: func() error { _, err := MarshalIntegerShardInventory(nil); return err }, wantErr: ErrIntegerBatchPlan},
		{name: "nil shard", run: func() error { _, err := MarshalIntegerShardManifest(nil); return err }, wantErr: ErrIntegerBatchPlan},
		{name: "nil batch", run: func() error { _, err := MarshalIntegerBatchManifest(nil); return err }, wantErr: ErrIntegerBatchPlan},
		{name: "component shape", run: func() error {
			manifest := integerJSONComponentFixture()
			manifest.TargetID = ""
			_, err := MarshalIntegerComponentManifest(&manifest)
			return err
		}, wantErr: ErrIntegerBatchPlan},
		{name: "component identity", run: func() error {
			manifest := integerJSONComponentFixture()
			manifest.SourceSHA = "invalid"
			_, err := MarshalIntegerComponentManifest(&manifest)
			return err
		}, wantErr: ErrIntegerBatchPlan},
		{name: "inventory shape", run: func() error {
			inventory := integerJSONInventoryFixture()
			inventory.Shard = ""
			_, err := MarshalIntegerShardInventory(&inventory)
			return err
		}, wantErr: ErrIntegerBatchPlan},
		{name: "inventory identity", run: func() error {
			inventory := integerJSONInventoryFixture()
			inventory.BatchID = "42-4"
			_, err := MarshalIntegerShardInventory(&inventory)
			return err
		}, wantErr: ErrIntegerBatchPlan},
		{name: "shard shape", run: func() error {
			manifest := integerJSONShardFixture("1")
			manifest.Shard = ""
			_, err := MarshalIntegerShardManifest(&manifest)
			return err
		}, wantErr: ErrIntegerBatchPlan},
		{name: "shard identity", run: func() error {
			manifest := integerJSONShardFixture("1")
			manifest.RunAttempt = 0
			_, err := MarshalIntegerShardManifest(&manifest)
			return err
		}, wantErr: ErrIntegerBatchPlan},
		{name: "shard artifact shape", run: func() error {
			manifest := integerJSONShardFixture("1")
			manifest.Artifact.Digest = "invalid"
			_, err := MarshalIntegerShardManifest(&manifest)
			return err
		}, wantErr: ErrIntegerBatchPlan},
		{name: "shard artifact identity", run: func() error {
			manifest := integerJSONShardFixture("1")
			manifest.Artifact.Name = "other-artifact"
			_, err := MarshalIntegerShardManifest(&manifest)
			return err
		}, wantErr: ErrIntegerIdentityMismatch},
		{name: "batch shape", run: func() error {
			manifest := integerJSONBatchFixture()
			manifest.SchemaVersion = 0
			_, err := MarshalIntegerBatchManifest(&manifest)
			return err
		}, wantErr: ErrIntegerBatchPlan},
		{name: "batch identity", run: func() error {
			manifest := integerJSONBatchFixture()
			manifest.PublicationID = "bad publication"
			_, err := MarshalIntegerBatchManifest(&manifest)
			return err
		}, wantErr: ErrIntegerBatchPlan},
		{name: "batch shard shape", run: func() error {
			manifest := integerJSONBatchFixture()
			manifest.Shards[0].Artifact.Digest = "invalid"
			_, err := MarshalIntegerBatchManifest(&manifest)
			return err
		}, wantErr: ErrIntegerBatchPlan},
		{name: "batch shard identity", run: func() error {
			manifest := integerJSONBatchFixture()
			manifest.Shards[0].Mode = IntegerBatchModeDelta
			_, err := MarshalIntegerBatchManifest(&manifest)
			return err
		}, wantErr: ErrIntegerIdentityMismatch},
		{name: "batch package artifact shape", run: func() error {
			manifest := integerJSONBatchFixture()
			manifest.Packages[0].Artifact.Digest = "invalid"
			_, err := MarshalIntegerBatchManifest(&manifest)
			return err
		}, wantErr: ErrIntegerBatchPlan},
		{name: "batch package artifact identity", run: func() error {
			manifest := integerJSONBatchFixture()
			manifest.Packages[0].Artifact.PublicationID = "other-publication"
			_, err := MarshalIntegerBatchManifest(&manifest)
			return err
		}, wantErr: ErrIntegerIdentityMismatch},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given
			// The table closure constructs one isolated invalid sentinel manifest.

			// When
			err := test.run()

			// Then
			require.ErrorIs(t, err, test.wantErr)
		})
	}
}
