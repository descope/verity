package ci

import "fmt"

func validateIntegerComponentManifest(manifest *IntegerComponentManifest) error {
	if manifest == nil || manifest.SchemaVersion != IntegerBatchSchemaVersion || manifest.TargetID == "" || manifest.Shard == "" {
		return fmt.Errorf("%w: component manifest shape", ErrIntegerBatchPlan)
	}
	return validateIntegerIdentity(integerIdentity{
		SourceSHA: manifest.SourceSHA, RunID: manifest.RunID, RunAttempt: manifest.RunAttempt,
		PublicationID: manifest.PublicationID, BatchID: manifest.BatchID,
	})
}

func validateIntegerShardInventory(inventory *IntegerShardInventory) error {
	if inventory == nil || inventory.SchemaVersion != IntegerBatchSchemaVersion || inventory.Shard == "" {
		return fmt.Errorf("%w: shard inventory shape", ErrIntegerBatchPlan)
	}
	return validateIntegerIdentity(integerIdentity{
		SourceSHA: inventory.SourceSHA, RunID: inventory.RunID, RunAttempt: inventory.RunAttempt,
		PublicationID: inventory.PublicationID, BatchID: inventory.BatchID,
	})
}

func validateIntegerShardManifest(manifest *IntegerShardManifest) error {
	if manifest == nil || manifest.SchemaVersion != IntegerBatchSchemaVersion || manifest.Shard == "" {
		return fmt.Errorf("%w: shard manifest shape", ErrIntegerBatchPlan)
	}
	if err := validateIntegerIdentity(integerIdentity{
		SourceSHA: manifest.SourceSHA, RunID: manifest.RunID, RunAttempt: manifest.RunAttempt,
		PublicationID: manifest.PublicationID, BatchID: manifest.BatchID,
	}); err != nil {
		return err
	}
	if err := validateIntegerArtifactRef(&manifest.Artifact); err != nil {
		return err
	}
	if manifest.Artifact.PublicationID != manifest.PublicationID || manifest.Artifact.Name != expectedIntegerShardArtifactName(manifest.PublicationID, manifest.Shard) {
		return fmt.Errorf("%w: shard artifact identity", ErrIntegerIdentityMismatch)
	}
	return nil
}

func validateIntegerBatchManifest(manifest *IntegerBatchManifest) error {
	if manifest == nil || manifest.SchemaVersion != IntegerBatchSchemaVersion {
		return fmt.Errorf("%w: batch manifest shape", ErrIntegerBatchPlan)
	}
	if err := validateIntegerIdentity(integerIdentity{
		SourceSHA: manifest.SourceSHA, RunID: manifest.RunID, RunAttempt: manifest.RunAttempt,
		PublicationID: manifest.PublicationID, BatchID: manifest.BatchID,
	}); err != nil {
		return err
	}
	if err := validateIntegerBatchManifestShards(manifest); err != nil {
		return err
	}
	return validateIntegerBatchManifestPackages(manifest)
}

func validateIntegerBatchManifestShards(manifest *IntegerBatchManifest) error {
	for index := range manifest.Shards {
		shard := &manifest.Shards[index]
		if err := validateIntegerShardManifest(shard); err != nil {
			return err
		}
		if !integerBatchIdentityMatches(manifest, shard) {
			return fmt.Errorf("%w: batch shard identity", ErrIntegerIdentityMismatch)
		}
	}
	return nil
}

func integerBatchIdentityMatches(manifest *IntegerBatchManifest, shard *IntegerShardManifest) bool {
	return shard.SourceSHA == manifest.SourceSHA && shard.RunID == manifest.RunID && shard.RunAttempt == manifest.RunAttempt &&
		shard.PublicationID == manifest.PublicationID && shard.BatchID == manifest.BatchID && shard.Mode == manifest.Mode && shard.Event == manifest.Event
}

func validateIntegerBatchManifestPackages(manifest *IntegerBatchManifest) error {
	for index := range manifest.Packages {
		if err := validateIntegerArtifactRef(&manifest.Packages[index].Artifact); err != nil {
			return err
		}
		if manifest.Packages[index].Artifact.PublicationID != manifest.PublicationID {
			return fmt.Errorf("%w: batch package artifact identity", ErrIntegerIdentityMismatch)
		}
	}
	return nil
}
