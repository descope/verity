package publication

import (
	"fmt"
	"path"
	"regexp"
	"slices"
	"strings"
)

var (
	shaPattern         = regexp.MustCompile(`^[0-9a-f]{40}([0-9a-f]{24})?$`)
	digestPattern      = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	componentPattern   = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
	artifactPattern    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	packageNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9+._-]*$`)
	fingerprintPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

func validateManifestShape(manifest *Manifest) error {
	if manifest.SchemaVersion != SchemaVersion {
		return invalidManifest("schema version %d", manifest.SchemaVersion)
	}
	if !shaPattern.MatchString(string(manifest.SourceSHA)) {
		return invalidManifest("source SHA %q", manifest.SourceSHA)
	}
	if manifest.RunID == 0 || manifest.RunAttempt == 0 || manifest.BatchID != BatchID(fmt.Sprintf("%d-%d", manifest.RunID, manifest.RunAttempt)) {
		return invalidManifest("run identity")
	}
	if err := validateModeAndBase(manifest); err != nil {
		return err
	}
	if err := validateComponents(manifest.Components); err != nil {
		return err
	}
	if !digestPattern.MatchString(string(manifest.SignerDigest)) {
		return invalidManifest("signer digest %q", manifest.SignerDigest)
	}
	if err := validateSigningKeyState(manifest); err != nil {
		return err
	}
	return validateOperations(manifest.Components, manifest.APKOperations)
}

func signingKeyStatePresent(manifest *Manifest) bool {
	return manifest.SigningKeyEpoch != 0 || manifest.ActiveSigningKeyFingerprint != "" ||
		manifest.TrustedSigningKeyFingerprints != nil || manifest.RevokedSigningKeyFingerprints != nil
}

func validateSigningKeyState(manifest *Manifest) error {
	if !signingKeyStatePresent(manifest) {
		return nil
	}
	if manifest.SigningKeyEpoch == 0 {
		return invalidManifest("signing key epoch must be positive")
	}
	if !fingerprintPattern.MatchString(manifest.ActiveSigningKeyFingerprint) {
		return invalidManifest("active signing key fingerprint %q", manifest.ActiveSigningKeyFingerprint)
	}
	if err := validateFingerprintSet("trusted signing key fingerprints", manifest.TrustedSigningKeyFingerprints); err != nil {
		return err
	}
	if err := validateFingerprintSet("revoked signing key fingerprints", manifest.RevokedSigningKeyFingerprints); err != nil {
		return err
	}
	if !slices.Contains(manifest.TrustedSigningKeyFingerprints, manifest.ActiveSigningKeyFingerprint) {
		return invalidManifest("active signing key is not trusted")
	}
	if slices.Contains(manifest.RevokedSigningKeyFingerprints, manifest.ActiveSigningKeyFingerprint) {
		return invalidManifest("active signing key is revoked")
	}
	trusted := make(map[string]struct{}, len(manifest.TrustedSigningKeyFingerprints))
	for _, fingerprint := range manifest.TrustedSigningKeyFingerprints {
		trusted[fingerprint] = struct{}{}
	}
	for _, fingerprint := range manifest.RevokedSigningKeyFingerprints {
		if _, exists := trusted[fingerprint]; exists {
			return invalidManifest("trusted and revoked signing key fingerprints overlap at %q", fingerprint)
		}
	}
	return nil
}

func validateFingerprintSet(name string, fingerprints []string) error {
	for index, fingerprint := range fingerprints {
		if !fingerprintPattern.MatchString(fingerprint) {
			return invalidManifest("%s contains invalid fingerprint %q", name, fingerprint)
		}
		if index > 0 && fingerprints[index-1] >= fingerprint {
			return invalidManifest("%s must be sorted and unique", name)
		}
	}
	return nil
}

func validateModeAndBase(manifest *Manifest) error {
	switch manifest.Mode {
	case ModeBootstrap:
		if manifest.PreviousManifestDigest != "" {
			return invalidManifest("bootstrap previous manifest digest")
		}
	case ModeSnapshot, ModeDelta, ModeRestore:
		if !digestPattern.MatchString(string(manifest.PreviousManifestDigest)) {
			return invalidManifest("previous manifest digest %q", manifest.PreviousManifestDigest)
		}
	default:
		return invalidManifest("mode %q", manifest.Mode)
	}
	return nil
}

func validateComponents(components []Component) error {
	if len(components) == 0 {
		return invalidManifest("components are required")
	}
	names := make(map[string]struct{}, len(components))
	artifacts := make(map[string]*Component, len(components))
	for index := range components {
		component := &components[index]
		if err := validateComponentFields(component); err != nil {
			return err
		}
		if _, exists := names[component.Name]; exists {
			return invalidManifest("duplicate component %q", component.Name)
		}
		if previous, exists := artifacts[component.ArtifactName]; exists {
			if !sameMultiArchitectureArtifact(previous, component) {
				return invalidManifest("duplicate artifact %q", component.ArtifactName)
			}
		} else {
			artifacts[component.ArtifactName] = component
		}
		names[component.Name] = struct{}{}
	}
	return nil
}

func validateComponentFields(component *Component) error {
	if !componentPattern.MatchString(component.Name) || !artifactPattern.MatchString(component.ArtifactName) {
		return invalidManifest("component name or artifact %q/%q", component.Name, component.ArtifactName)
	}
	if component.Kind != ComponentKindAPK && component.Kind != ComponentKindGeneric {
		return invalidManifest("component %q kind %q", component.Name, component.Kind)
	}
	if err := validateComponentArchitecture(component); err != nil {
		return err
	}
	if !digestPattern.MatchString(string(component.ArtifactDigest)) || !validWorkflow(component.Workflow) {
		return invalidManifest("component %q digest or workflow", component.Name)
	}
	if component.ManifestDigest != "" && !digestPattern.MatchString(string(component.ManifestDigest)) {
		return invalidManifest("component %q manifest digest", component.Name)
	}
	if !validEvent(component.Event) || component.Result != ResultSuccess {
		return invalidManifest("component %q event or result", component.Name)
	}
	return nil
}

func sameMultiArchitectureArtifact(left, right *Component) bool {
	return left.Kind == ComponentKindAPK && right.Kind == ComponentKindAPK &&
		left.Architecture != right.Architecture &&
		left.ArtifactDigest == right.ArtifactDigest && left.ManifestDigest == right.ManifestDigest &&
		left.Workflow == right.Workflow && left.Event == right.Event && left.Result == right.Result
}

func validateComponentArchitecture(component *Component) error {
	switch component.Kind {
	case ComponentKindAPK:
		if component.Architecture != ArchitectureX8664 && component.Architecture != ArchitectureAArch64 {
			return invalidManifest("APK component %q architecture %q", component.Name, component.Architecture)
		}
	case ComponentKindGeneric:
		if component.Architecture != "" {
			return invalidManifest("generic component %q declares architecture %q", component.Name, component.Architecture)
		}
	}
	return nil
}

func validateOperations(components []Component, operations []APKOperation) error {
	if operations == nil {
		return invalidManifest("apk_operations must be an array")
	}
	artifacts := make(map[string][]*Component, len(components))
	for index := range components {
		component := &components[index]
		artifacts[component.ArtifactName] = append(artifacts[component.ArtifactName], component)
	}
	seen := make(map[string]struct{}, len(operations))
	for _, operation := range operations {
		if operation.Architecture != ArchitectureX8664 && operation.Architecture != ArchitectureAArch64 {
			return invalidManifest("operation architecture %q", operation.Architecture)
		}
		if !packageNamePattern.MatchString(operation.PackageName) {
			return invalidManifest("operation package %q", operation.PackageName)
		}
		key := string(operation.Architecture) + "/" + operation.PackageName
		if _, exists := seen[key]; exists {
			return invalidManifest("duplicate operation %q", key)
		}
		seen[key] = struct{}{}
		if err := validateOperationArtifact(&operation, key, artifacts); err != nil {
			return err
		}
	}
	return nil
}

func validateOperationArtifact(operation *APKOperation, key string, artifacts map[string][]*Component) error {
	switch operation.Action {
	case APKUpsert:
		if !artifactPattern.MatchString(operation.ArtifactName) || !digestPattern.MatchString(string(operation.ArtifactDigest)) {
			return invalidManifest("upsert %q artifact", key)
		}
		components, exists := artifacts[operation.ArtifactName]
		if !exists {
			return invalidManifest("upsert %q unknown artifact %q", key, operation.ArtifactName)
		}
		var component *Component
		for _, candidate := range components {
			if candidate.Architecture == operation.Architecture {
				component = candidate
				break
			}
		}
		if component == nil {
			return invalidManifest("upsert %q architecture does not match artifact %q", key, operation.ArtifactName)
		}
		if component.Kind != ComponentKindAPK {
			return invalidManifest("upsert %q references non-APK component %q", key, component.Name)
		}
		if operation.ArtifactDigest != component.ArtifactDigest {
			return invalidManifest("upsert %q digest does not match component artifact %q", key, operation.ArtifactName)
		}
	case APKRemove:
		if operation.ArtifactName != "" || operation.ArtifactDigest != "" {
			return invalidManifest("remove %q carries artifact data", key)
		}
	default:
		return invalidManifest("operation action %q", operation.Action)
	}
	return nil
}

func validWorkflow(workflow string) bool {
	if !strings.HasPrefix(workflow, ".github/workflows/") || strings.Contains(workflow, "\\") {
		return false
	}
	clean := path.Clean(workflow)
	return clean == workflow && !strings.Contains(workflow, "../") && (strings.HasSuffix(workflow, ".yaml") || strings.HasSuffix(workflow, ".yml"))
}

func validEvent(event Event) bool {
	switch event {
	case EventSchedule, EventPush, EventWorkflowCall, EventWorkflowDispatch:
		return true
	default:
		return false
	}
}

func invalidManifest(format string, values ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidManifest, fmt.Sprintf(format, values...))
}
