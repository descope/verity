package ci

import (
	"fmt"
	"sort"
)

func AggregateIntegerBatch(plan *IntegerBatchPlan, shards []IntegerShardManifest) (IntegerBatchManifest, error) {
	if err := validateIntegerBatchPlan(plan); err != nil {
		return IntegerBatchManifest{}, err
	}
	aggregation := newIntegerBatchAggregation(plan)
	for index := range shards {
		if err := aggregation.addShard(&shards[index]); err != nil {
			return IntegerBatchManifest{}, err
		}
	}
	if err := aggregation.validateComplete(); err != nil {
		return IntegerBatchManifest{}, err
	}
	return aggregation.manifest(shards), nil
}

type integerBatchAggregation struct {
	plan             *IntegerBatchPlan
	expectedShards   []string
	expectedShardSet map[string]struct{}
	seenShards       map[string]struct{}
	published        map[string]IntegerPublishedPackage
}

func newIntegerBatchAggregation(plan *IntegerBatchPlan) *integerBatchAggregation {
	expectedShards := sortedIntegerShardIDs(plan)
	expectedShardSet := make(map[string]struct{}, len(expectedShards))
	for _, shard := range expectedShards {
		expectedShardSet[shard] = struct{}{}
	}
	return &integerBatchAggregation{
		plan: plan, expectedShards: expectedShards, expectedShardSet: expectedShardSet,
		seenShards: make(map[string]struct{}, len(expectedShards)),
		published:  make(map[string]IntegerPublishedPackage, len(plan.Packages)),
	}
}

func (aggregation *integerBatchAggregation) addShard(shard *IntegerShardManifest) error {
	if err := validateIntegerShardIdentity(aggregation.plan, shard); err != nil {
		return err
	}
	if _, expected := aggregation.expectedShardSet[shard.Shard]; !expected {
		return fmt.Errorf("%w: unexpected shard %s", ErrIntegerBatchIncomplete, shard.Shard)
	}
	if _, exists := aggregation.seenShards[shard.Shard]; exists {
		return fmt.Errorf("%w: shard %s", ErrIntegerPackageDuplicate, shard.Shard)
	}
	aggregation.seenShards[shard.Shard] = struct{}{}
	for _, pkg := range shard.Packages {
		key := string(pkg.Architecture) + "/" + pkg.Name
		if _, exists := aggregation.published[key]; exists {
			return fmt.Errorf("%w: package %s", ErrIntegerPackageDuplicate, key)
		}
		if !integerPlanContainsPackage(aggregation.plan, shard.Shard, pkg) {
			return fmt.Errorf("%w: package %s", ErrIntegerPackageUndeclared, key)
		}
		aggregation.published[key] = IntegerPublishedPackage{
			Architecture: pkg.Architecture,
			Name:         pkg.Name,
			SHA256:       pkg.SHA256,
			Artifact:     shard.Artifact,
		}
	}
	return nil
}

func (aggregation *integerBatchAggregation) validateComplete() error {
	for _, shard := range aggregation.expectedShards {
		if _, exists := aggregation.seenShards[shard]; !exists {
			return fmt.Errorf("%w: shard %s", ErrIntegerBatchIncomplete, shard)
		}
	}
	for _, planned := range aggregation.plan.Packages {
		key := string(planned.Architecture) + "/" + planned.Name
		if _, exists := aggregation.published[key]; !exists {
			return fmt.Errorf("%w: package %s", ErrIntegerBatchIncomplete, key)
		}
	}
	return nil
}

func (aggregation *integerBatchAggregation) manifest(shards []IntegerShardManifest) IntegerBatchManifest {
	packages := make([]IntegerPublishedPackage, 0, len(aggregation.published))
	for _, pkg := range aggregation.published {
		packages = append(packages, pkg)
	}
	sort.Slice(packages, func(i, j int) bool {
		if packages[i].Architecture != packages[j].Architecture {
			return packages[i].Architecture < packages[j].Architecture
		}
		return packages[i].Name < packages[j].Name
	})
	canonicalShards := append([]IntegerShardManifest(nil), shards...)
	sort.Slice(canonicalShards, func(i, j int) bool { return canonicalShards[i].Shard < canonicalShards[j].Shard })
	return IntegerBatchManifest{
		SchemaVersion: IntegerBatchSchemaVersion,
		SourceSHA:     aggregation.plan.SourceSHA,
		RunID:         aggregation.plan.RunID,
		RunAttempt:    aggregation.plan.RunAttempt,
		PublicationID: aggregation.plan.PublicationID,
		BatchID:       aggregation.plan.BatchID,
		Mode:          aggregation.plan.Mode,
		Event:         aggregation.plan.Event,
		Shards:        canonicalShards,
		Packages:      packages,
	}
}

func validateIntegerShardIdentity(plan *IntegerBatchPlan, shard *IntegerShardManifest) error {
	if err := validateIntegerArtifactRef(&shard.Artifact); err != nil {
		return fmt.Errorf("%w: shard %s artifact", ErrIntegerIdentityMismatch, shard.Shard)
	}
	if shard.SchemaVersion != IntegerBatchSchemaVersion || shard.SourceSHA != plan.SourceSHA ||
		shard.RunID != plan.RunID || shard.RunAttempt != plan.RunAttempt || shard.PublicationID != plan.PublicationID || shard.BatchID != plan.BatchID ||
		shard.Mode != plan.Mode || shard.Event != plan.Event || shard.Shard == "" ||
		shard.Artifact.PublicationID != plan.PublicationID || shard.Artifact.Name != expectedIntegerShardArtifactName(plan.PublicationID, shard.Shard) || !integerDigestPattern.MatchString(shard.Artifact.Digest) {
		return fmt.Errorf("%w: shard %s", ErrIntegerIdentityMismatch, shard.Shard)
	}
	return nil
}

func integerPlanContainsPackage(plan *IntegerBatchPlan, shard string, pkg IntegerPackageFile) bool {
	for _, planned := range plan.Packages {
		if planned.Architecture != pkg.Architecture || planned.Name != pkg.Name {
			continue
		}
		target, ok := findIntegerTarget(plan.Targets, planned.Producer)
		return ok && target.Shard == shard
	}
	return false
}

func errorsJoin(primary, secondary error) error {
	if primary != nil {
		return primary
	}
	return secondary
}
