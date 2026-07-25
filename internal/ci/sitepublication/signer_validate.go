package sitepublication

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/verity-org/verity/internal/ci/publication"
	"github.com/verity-org/verity/internal/ci/signerlock"
)

func ValidateSignerPlan(plan *SignerPlan) error {
	if err := validateSignerIdentity(plan); err != nil {
		return err
	}
	if err := validateSignerExecutionSpec(&plan.Execution); err != nil {
		return err
	}
	workspace, keyDirectory, err := validateSignerDirectories(plan.Execution.WorkspaceDir, plan.Cleanup.KeyDirectory)
	if err != nil || workspace != plan.Execution.WorkspaceDir || keyDirectory != plan.Cleanup.KeyDirectory {
		return fmt.Errorf("%w: cleanup directories", ErrInvalidSignerPlan)
	}
	if plan.Cleanup.KeyPath != filepath.Join(plan.Cleanup.KeyDirectory, "verity.rsa") {
		return fmt.Errorf("%w: cleanup path", ErrInvalidSignerPlan)
	}
	if _, err := validateSignerFilesystem(plan, false); err != nil {
		return err
	}
	if err := VerifySignerInputs(plan.Execution.WorkspaceDir, &plan.Authorization, plan.InputDigest, plan.PublicationPlanDigest, plan.ManifestDigest); err != nil {
		return err
	}
	expected, err := buildSignerSteps(plan)
	if err != nil {
		return err
	}
	if !equalSignerSteps(plan.Steps, expected) {
		return fmt.Errorf("%w: execution commands differ from closed signer specification", ErrInvalidSignerPlan)
	}
	return nil
}

func validateSignerIdentity(plan *SignerPlan) error {
	if plan == nil || plan.SchemaVersion != SchemaVersion {
		return fmt.Errorf("%w: schema", ErrInvalidSignerPlan)
	}
	if !digestPattern.MatchString(string(plan.PublicationPlanDigest)) || !digestPattern.MatchString(string(plan.ManifestDigest)) ||
		!digestPattern.MatchString(string(plan.InputDigest)) || !digestPattern.MatchString(string(plan.SignerDigest)) {
		return fmt.Errorf("%w: digest", ErrInvalidSignerPlan)
	}
	if !sourcePattern.MatchString(string(plan.SignerSourceSHA)) {
		return fmt.Errorf("%w: signer source SHA", ErrInvalidSignerPlan)
	}
	if plan.Authorization.PublicationPlanDigest != plan.PublicationPlanDigest || plan.Authorization.ManifestDigest != plan.ManifestDigest ||
		plan.Authorization.Mode != plan.Execution.Mode {
		return fmt.Errorf("%w: input authorization identity", ErrInvalidSignerPlan)
	}
	if plan.ImageReference != signerlock.SignerImageRepository+"@"+string(plan.SignerDigest) {
		return fmt.Errorf("%w: image reference", ErrInvalidSignerPlan)
	}
	return nil
}

func validateSignerExecutionSpec(spec *SignerExecutionSpec) error {
	if _, err := trustedRuntimeBinary(spec.Runtime); err != nil {
		return err
	}
	if err := validateSignerRepository(spec.Repository); err != nil {
		return err
	}
	if !digestPattern.MatchString(string(spec.PathSnapshot)) {
		return fmt.Errorf("%w: path snapshot", ErrInvalidSignerPlan)
	}
	for name, value := range map[string]string{
		"manifest": spec.ManifestPath, "packages": spec.PackagesPath,
		"output": spec.OutputAPKPath, "public key": spec.PublicKeyPath,
	} {
		if err := validateCanonicalSignerPath(name, value); err != nil {
			return err
		}
	}
	switch spec.Mode {
	case publication.ModeBootstrap, publication.ModeSnapshot:
		if spec.BaseAPKPath != "" || spec.DeltaManifestPath != "" {
			return fmt.Errorf("%w: unexpected delta paths", ErrInvalidSignerPlan)
		}
	case publication.ModeDelta:
		if err := validateCanonicalSignerPath("base APK", spec.BaseAPKPath); err != nil {
			return err
		}
		if err := validateCanonicalSignerPath("delta manifest", spec.DeltaManifestPath); err != nil {
			return err
		}
	case publication.ModeRestore:
		return ErrUnsupportedSignMode
	default:
		return fmt.Errorf("%w: mode %q", ErrInvalidSignerPlan, spec.Mode)
	}
	if err := validateSignerDataPaths(spec); err != nil {
		return err
	}
	return nil
}

func validateSignerDataPaths(spec *SignerExecutionSpec) error {
	paths := []struct {
		name  string
		value string
	}{
		{name: "manifest", value: spec.ManifestPath},
		{name: "packages", value: spec.PackagesPath},
		{name: "output", value: spec.OutputAPKPath},
		{name: "public key", value: spec.PublicKeyPath},
	}
	if spec.Mode == publication.ModeDelta {
		paths = append(
			paths,
			struct {
				name  string
				value string
			}{name: "base APK", value: spec.BaseAPKPath},
			struct {
				name  string
				value string
			}{name: "delta manifest", value: spec.DeltaManifestPath},
		)
	}
	for index, path := range paths {
		for _, other := range paths[index+1:] {
			if signerPathsOverlap(path.value, other.value) {
				return fmt.Errorf("%w: %s and %s paths alias", ErrInvalidSignerPlan, path.name, other.name)
			}
		}
	}
	return nil
}

func signerPathsOverlap(first, second string) bool {
	return first == second || insideDirectory(first, second) || insideDirectory(second, first)
}

func validateCanonicalSignerPath(name, value string) error {
	clean, err := cleanSignerPath(value)
	if err != nil || clean != value {
		return fmt.Errorf("%w: non-canonical %s path", ErrInvalidSignerPlan, name)
	}
	return nil
}

type dockerMountSpec struct {
	source      string
	destination string
	readonly    bool
}

type dockerMountFields struct {
	typeSet, sourceSet, destinationSet, readonlySet bool
}

func parseDockerMountArgument(argument string) (dockerMountSpec, error) {
	if !strings.HasPrefix(argument, "--mount=") {
		return dockerMountSpec{}, fmt.Errorf("%w: invalid mount argument", ErrInvalidSignerPlan)
	}
	fields := strings.Split(strings.TrimPrefix(argument, "--mount="), ",")
	spec := dockerMountSpec{}
	seen := dockerMountFields{}
	for _, field := range fields {
		if err := parseDockerMountField(field, &spec, &seen); err != nil {
			return dockerMountSpec{}, err
		}
	}
	if !seen.typeSet || !seen.sourceSet || !seen.destinationSet || spec.source == "" || spec.destination == "" {
		return dockerMountSpec{}, fmt.Errorf("%w: mount source and destination are required", ErrInvalidSignerPlan)
	}
	return spec, nil
}

func parseDockerMountField(field string, spec *dockerMountSpec, seen *dockerMountFields) error {
	if field == "readonly" {
		if seen.readonlySet {
			return fmt.Errorf("%w: duplicate readonly mount field", ErrInvalidSignerPlan)
		}
		seen.readonlySet = true
		spec.readonly = true
		return nil
	}
	key, value, found := strings.Cut(field, "=")
	if !found || value == "" {
		return fmt.Errorf("%w: invalid mount field %q", ErrInvalidSignerPlan, field)
	}
	switch key {
	case "type":
		if seen.typeSet {
			return fmt.Errorf("%w: duplicate mount type", ErrInvalidSignerPlan)
		}
		seen.typeSet = true
		if value != "bind" {
			return fmt.Errorf("%w: mount type %q", ErrInvalidSignerPlan, value)
		}
	case "src":
		if seen.sourceSet {
			return fmt.Errorf("%w: duplicate mount source", ErrInvalidSignerPlan)
		}
		seen.sourceSet = true
		spec.source = value
	case "dst":
		if seen.destinationSet {
			return fmt.Errorf("%w: duplicate mount destination", ErrInvalidSignerPlan)
		}
		seen.destinationSet = true
		spec.destination = value
	default:
		return fmt.Errorf("%w: unsupported mount field %q", ErrInvalidSignerPlan, key)
	}
	return nil
}

func equalSignerSteps(actual, expected []SignerStep) bool {
	if len(actual) != len(expected) {
		return false
	}
	for index := range expected {
		if actual[index].Name != expected[index].Name || actual[index].KeyAccess != expected[index].KeyAccess ||
			actual[index].Command.Name != expected[index].Command.Name ||
			!slices.Equal(actual[index].Command.Args, expected[index].Command.Args) {
			return false
		}
	}
	return true
}
