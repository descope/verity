package ci

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
)

var (
	integerSourceSHAPattern     = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)
	integerPublicationIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
	integerDigestPattern        = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	integerArtifactPattern      = regexp.MustCompile(`^[A-Za-z0-9._-]+-[0-9a-f]{12}$`)
	integerArtifactNamePattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,255}$`)
)

type integerIdentity struct {
	SourceSHA     string
	RunID         uint64
	RunAttempt    uint64
	PublicationID string
	BatchID       string
}

func validateIntegerProductionOptions(options *IntegerProductionOptions) error {
	if options == nil {
		return fmt.Errorf("%w: options are required", ErrIntegerBatchPlan)
	}
	if err := validateIntegerIdentity(integerIdentity{
		SourceSHA: options.SourceSHA, RunID: options.RunID, RunAttempt: options.RunAttempt,
		PublicationID: options.PublicationID, BatchID: options.BatchID,
	}); err != nil {
		return err
	}
	return nil
}

func validateIntegerBatchPlan(plan *IntegerBatchPlan) error {
	if plan == nil || plan.SchemaVersion != IntegerBatchSchemaVersion {
		return fmt.Errorf("%w: schema version", ErrIntegerBatchPlan)
	}
	if err := validateIntegerIdentity(integerIdentity{
		SourceSHA: plan.SourceSHA, RunID: plan.RunID, RunAttempt: plan.RunAttempt,
		PublicationID: plan.PublicationID, BatchID: plan.BatchID,
	}); err != nil {
		return err
	}
	if plan.Mode != IntegerBatchModeSnapshot && plan.Mode != IntegerBatchModeDelta {
		return fmt.Errorf("%w: mode %q", ErrIntegerBatchPlan, plan.Mode)
	}
	if !validIntegerBatchEvent(plan.Event) {
		return fmt.Errorf("%w: event %q", ErrIntegerBatchPlan, plan.Event)
	}
	return validateIntegerPlanEntries(plan)
}

func validateIntegerIdentity(identity integerIdentity) error {
	switch {
	case !integerSourceSHAPattern.MatchString(identity.SourceSHA):
		return fmt.Errorf("%w: source SHA is required", ErrIntegerBatchPlan)
	case identity.RunID == 0 || identity.RunAttempt == 0:
		return fmt.Errorf("%w: run identity is required", ErrIntegerBatchPlan)
	case !integerPublicationIDPattern.MatchString(identity.PublicationID):
		return fmt.Errorf("%w: publication ID is required", ErrIntegerBatchPlan)
	case identity.BatchID != fmt.Sprintf("%d-%d", identity.RunID, identity.RunAttempt):
		return fmt.Errorf("%w: batch %q does not match run identity", ErrIntegerBatchPlan, identity.BatchID)
	case identity.PublicationID == identity.BatchID:
		return fmt.Errorf("%w: publication ID must differ from batch ID", ErrIntegerBatchPlan)
	default:
		return nil
	}
}

func validateIntegerArtifactRef(artifact *IntegerArtifactRef) error {
	if artifact == nil || !integerPublicationIDPattern.MatchString(artifact.PublicationID) ||
		!integerArtifactNamePattern.MatchString(artifact.Name) || !integerDigestPattern.MatchString(artifact.Digest) {
		return fmt.Errorf("%w: invalid Integer artifact reference", ErrIntegerBatchPlan)
	}
	return nil
}

func expectedIntegerShardArtifactName(publicationID, shard string) string {
	return "apk-repository-" + publicationID + "-" + shard
}

func validateIntegerPlanEntries(plan *IntegerBatchPlan) error {
	targets := make(map[string]*IntegerBatchTarget, len(plan.Targets))
	artifactKeys := make(map[string]struct{}, len(plan.Targets))
	for index := range plan.Targets {
		target := &plan.Targets[index]
		if target.Name == "" || target.Version == "" || target.Type == "" || target.Shard == "" ||
			!integerArtifactPattern.MatchString(target.ArtifactKey) {
			return fmt.Errorf("%w: incomplete target", ErrIntegerBatchPlan)
		}
		if _, exists := targets[target.ID()]; exists {
			return fmt.Errorf("%w: target %s", ErrIntegerPackageDuplicate, target.ID())
		}
		targets[target.ID()] = target
		if _, exists := artifactKeys[target.ArtifactKey]; exists {
			return fmt.Errorf("%w: artifact key %s", ErrIntegerPackageDuplicate, target.ArtifactKey)
		}
		artifactKeys[target.ArtifactKey] = struct{}{}
		if err := validateIntegerTargetPackages(target); err != nil {
			return err
		}
	}
	return validateIntegerPlannedPackages(plan.Packages, targets)
}

func validateIntegerPlannedPackages(packages []IntegerPlannedPackage, targets map[string]*IntegerBatchTarget) error {
	seen := map[string]struct{}{}
	for _, pkg := range packages {
		if !validIntegerArchitecture(pkg.Architecture) || !integerPackageNamePattern.MatchString(pkg.Name) {
			return fmt.Errorf("%w: invalid package declaration", ErrIntegerBatchPlan)
		}
		target, exists := targets[pkg.Producer]
		if !exists || !containsString(target.PublishPackages, pkg.Name) {
			return fmt.Errorf("%w: package %s has invalid producer %s", ErrIntegerBatchPlan, pkg.Name, pkg.Producer)
		}
		key := string(pkg.Architecture) + "/" + pkg.Name
		if _, exists := seen[key]; exists {
			return fmt.Errorf("%w: package %s", ErrIntegerPackageDuplicate, key)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func validateIntegerTargetPackages(target *IntegerBatchTarget) error {
	expected := append([]string(nil), target.ExpectedPackages...)
	publish := append([]string(nil), target.PublishPackages...)
	sort.Strings(expected)
	sort.Strings(publish)
	for index, name := range expected {
		if !integerPackageNamePattern.MatchString(name) || index > 0 && expected[index-1] == name {
			return fmt.Errorf("%w: target %s expected package %q", ErrIntegerBatchPlan, target.ID(), name)
		}
	}
	for index, name := range publish {
		if index > 0 && publish[index-1] == name {
			return fmt.Errorf("%w: target %s publish package %q", ErrIntegerBatchPlan, target.ID(), name)
		}
		if !containsString(expected, name) {
			return fmt.Errorf("%w: target %s publishes undeclared package %q", ErrIntegerBatchPlan, target.ID(), name)
		}
	}
	return nil
}

func validIntegerBatchEvent(event IntegerBatchEvent) bool {
	switch event {
	case IntegerBatchEventSchedule, IntegerBatchEventPush, IntegerBatchEventWorkflowCall, IntegerBatchEventWorkflowDispatch:
		return true
	default:
		return false
	}
}

func validIntegerArchitecture(architecture IntegerArchitecture) bool {
	return architecture == IntegerArchitectureX8664 || architecture == IntegerArchitectureAArch64
}

func containsString(values []string, wanted string) bool {
	return slices.Contains(values, wanted)
}
