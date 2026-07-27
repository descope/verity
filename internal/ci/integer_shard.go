package ci

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

func AggregateIntegerShard(ctx context.Context, options *IntegerShardOptions) (IntegerShardInventory, error) {
	if err := ctx.Err(); err != nil {
		return IntegerShardInventory{}, fmt.Errorf("aggregate Integer shard: %w", err)
	}
	aggregation, err := prepareIntegerShardAggregation(options)
	if err != nil {
		return IntegerShardInventory{}, err
	}
	if err := aggregation.loadComponents(); err != nil {
		return IntegerShardInventory{}, err
	}
	return aggregation.buildInventory(ctx)
}

type integerShardAggregation struct {
	options         *IntegerShardOptions
	expectedTargets map[string]struct{}
	components      map[string]*IntegerComponentManifest
	componentRoots  map[string]string
}

func prepareIntegerShardAggregation(options *IntegerShardOptions) (*integerShardAggregation, error) {
	if options == nil || options.Plan == nil || options.Shard == "" || options.OutputDir == "" {
		return nil, fmt.Errorf("%w: shard options are incomplete", ErrIntegerBatchPlan)
	}
	if err := validateIntegerBatchPlan(options.Plan); err != nil {
		return nil, err
	}
	return &integerShardAggregation{
		options:         options,
		expectedTargets: expectedIntegerComponentTargets(options.Plan, options.Shard),
		components:      make(map[string]*IntegerComponentManifest, len(options.ComponentDirs)),
		componentRoots:  make(map[string]string, len(options.ComponentDirs)),
	}, nil
}

func (aggregation *integerShardAggregation) loadComponents() error {
	for _, root := range aggregation.options.ComponentDirs {
		data, err := os.ReadFile(filepath.Join(root, IntegerComponentManifestName))
		if err != nil {
			return fmt.Errorf("read Integer component manifest: %w", err)
		}
		component, err := ParseIntegerComponentManifest(data)
		if err != nil {
			return err
		}
		if err := validateIntegerComponentIdentity(aggregation.options.Plan, aggregation.options.Shard, &component); err != nil {
			return err
		}
		if _, expected := aggregation.expectedTargets[component.TargetID]; !expected {
			return fmt.Errorf("%w: unexpected target %s", ErrIntegerShardIncomplete, component.TargetID)
		}
		if _, exists := aggregation.components[component.TargetID]; exists {
			return fmt.Errorf("%w: component %s", ErrIntegerPackageDuplicate, component.TargetID)
		}
		aggregation.components[component.TargetID] = &component
		aggregation.componentRoots[component.TargetID] = root
	}
	for targetID := range aggregation.expectedTargets {
		if _, exists := aggregation.components[targetID]; !exists {
			return fmt.Errorf("%w: target %s", ErrIntegerShardIncomplete, targetID)
		}
	}
	return nil
}

func (aggregation *integerShardAggregation) buildInventory(ctx context.Context) (IntegerShardInventory, error) {
	if err := os.RemoveAll(aggregation.options.OutputDir); err != nil {
		return IntegerShardInventory{}, fmt.Errorf("clear shard output: %w", err)
	}
	inventory := IntegerShardInventory{
		SchemaVersion: IntegerBatchSchemaVersion,
		SourceSHA:     aggregation.options.Plan.SourceSHA,
		RunID:         aggregation.options.Plan.RunID,
		RunAttempt:    aggregation.options.Plan.RunAttempt,
		PublicationID: aggregation.options.Plan.PublicationID,
		BatchID:       aggregation.options.Plan.BatchID,
		Mode:          aggregation.options.Plan.Mode,
		Event:         aggregation.options.Plan.Event,
		Shard:         aggregation.options.Shard,
	}
	seen := map[string]struct{}{}
	for targetID, component := range aggregation.components {
		root := aggregation.componentRoots[targetID]
		for _, pkg := range component.Packages {
			key := string(pkg.Architecture) + "/" + pkg.Name
			if _, exists := seen[key]; exists {
				return IntegerShardInventory{}, fmt.Errorf("%w: package %s", ErrIntegerPackageDuplicate, key)
			}
			seen[key] = struct{}{}
			request := integerComponentPackageCopy{
				options: aggregation.options, targetID: targetID, root: root, pkg: pkg,
			}
			if err := request.copy(ctx); err != nil {
				return IntegerShardInventory{}, err
			}
			inventory.Packages = append(inventory.Packages, pkg)
		}
	}
	if err := validateIntegerShardPackageCompleteness(aggregation.options.Plan, aggregation.options.Shard, inventory.Packages); err != nil {
		return IntegerShardInventory{}, err
	}
	sortIntegerPackageFiles(inventory.Packages)
	return inventory, nil
}

func FinalizeIntegerShard(inventory *IntegerShardInventory, artifact IntegerArtifactRef) (IntegerShardManifest, error) {
	if err := validateIntegerShardInventory(inventory); err != nil {
		return IntegerShardManifest{}, err
	}
	if err := validateIntegerArtifactRef(&artifact); err != nil {
		return IntegerShardManifest{}, fmt.Errorf("%w: invalid shard artifact", ErrIntegerBatchPlan)
	}
	if artifact.PublicationID != inventory.PublicationID {
		return IntegerShardManifest{}, fmt.Errorf("%w: shard artifact publication identity", ErrIntegerIdentityMismatch)
	}
	expectedName := expectedIntegerShardArtifactName(inventory.PublicationID, inventory.Shard)
	if artifact.Name != expectedName {
		return IntegerShardManifest{}, fmt.Errorf("%w: want artifact %s, got %s", ErrIntegerIdentityMismatch, expectedName, artifact.Name)
	}
	return IntegerShardManifest{
		SchemaVersion: inventory.SchemaVersion,
		SourceSHA:     inventory.SourceSHA,
		RunID:         inventory.RunID,
		RunAttempt:    inventory.RunAttempt,
		PublicationID: inventory.PublicationID,
		BatchID:       inventory.BatchID,
		Mode:          inventory.Mode,
		Event:         inventory.Event,
		Shard:         inventory.Shard,
		Artifact:      artifact,
		Packages:      append([]IntegerPackageFile(nil), inventory.Packages...),
	}, nil
}

func expectedIntegerComponentTargets(plan *IntegerBatchPlan, shard string) map[string]struct{} {
	expected := map[string]struct{}{}
	for index := range plan.Targets {
		target := &plan.Targets[index]
		if target.Shard == shard && len(target.PublishPackages) > 0 {
			expected[target.ID()] = struct{}{}
		}
	}
	return expected
}

func validateIntegerComponentIdentity(plan *IntegerBatchPlan, shard string, component *IntegerComponentManifest) error {
	if component.SchemaVersion != IntegerBatchSchemaVersion || component.SourceSHA != plan.SourceSHA ||
		component.RunID != plan.RunID || component.RunAttempt != plan.RunAttempt || component.PublicationID != plan.PublicationID || component.BatchID != plan.BatchID ||
		component.Mode != plan.Mode || component.Event != plan.Event || component.Shard != shard {
		return fmt.Errorf("%w: component %s", ErrIntegerIdentityMismatch, component.TargetID)
	}
	return nil
}

type integerComponentPackageCopy struct {
	options  *IntegerShardOptions
	targetID string
	root     string
	pkg      IntegerPackageFile
}

func (request *integerComponentPackageCopy) copy(ctx context.Context) error {
	if err := request.validate(); err != nil {
		return err
	}
	source := filepath.Join(request.root, filepath.FromSlash(request.pkg.Path))
	identity, err := inspectIntegerAPK(source)
	if err != nil {
		return err
	}
	if identity.architecture != request.pkg.Architecture || identity.name != request.pkg.Name {
		return fmt.Errorf("%w: component package %s", ErrIntegerPackageArchitecture, request.pkg.Path)
	}
	if identity.digest != request.pkg.SHA256 {
		return fmt.Errorf("%w: package digest %s", ErrIntegerIdentityMismatch, request.pkg.Path)
	}
	destination := filepath.Join(request.options.OutputDir, filepath.FromSlash(request.pkg.Path))
	return copyIntegerFile(ctx, source, destination)
}

func (request *integerComponentPackageCopy) validate() error {
	if !validIntegerArchitecture(request.pkg.Architecture) || !integerPackageNamePattern.MatchString(request.pkg.Name) ||
		!integerDigestPattern.MatchString(request.pkg.SHA256) {
		return fmt.Errorf("%w: invalid component package", ErrIntegerBatchPlan)
	}
	if !integerPlanContainsProducedPackage(request.options.Plan, request.targetID, request.pkg) {
		return fmt.Errorf("%w: %s/%s", ErrIntegerPackageUndeclared, request.pkg.Architecture, request.pkg.Name)
	}
	clean := filepath.ToSlash(filepath.Clean(request.pkg.Path))
	if clean != request.pkg.Path || filepath.IsAbs(request.pkg.Path) || clean == "." || clean == ".." {
		return fmt.Errorf("%w: unsafe package path %q", ErrIntegerBatchPlan, request.pkg.Path)
	}
	return nil
}

func integerPlanContainsProducedPackage(plan *IntegerBatchPlan, targetID string, pkg IntegerPackageFile) bool {
	for _, candidate := range plan.Packages {
		if candidate.Architecture == pkg.Architecture && candidate.Name == pkg.Name && candidate.Producer == targetID {
			return true
		}
	}
	return false
}

func validateIntegerShardPackageCompleteness(plan *IntegerBatchPlan, shard string, packages []IntegerPackageFile) error {
	found := map[string]struct{}{}
	for _, pkg := range packages {
		found[string(pkg.Architecture)+"/"+pkg.Name] = struct{}{}
	}
	for _, planned := range plan.Packages {
		target, ok := findIntegerTarget(plan.Targets, planned.Producer)
		if !ok || target.Shard != shard {
			continue
		}
		key := string(planned.Architecture) + "/" + planned.Name
		if _, exists := found[key]; !exists {
			return fmt.Errorf("%w: package %s", ErrIntegerShardIncomplete, key)
		}
	}
	return nil
}

func sortedIntegerShardIDs(plan *IntegerBatchPlan) []string {
	set := map[string]struct{}{}
	for index := range plan.Targets {
		set[plan.Targets[index].Shard] = struct{}{}
	}
	ids := make([]string, 0, len(set))
	for id := range set {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
