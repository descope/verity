package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

var (
	// ErrMissingName is returned when an image definition has no name.
	ErrMissingName = errors.New("missing required field: name")

	// ErrMissingPackage is returned when an image definition has no upstream.package.
	ErrMissingPackage = errors.New("missing required field: upstream.package")

	// ErrNoTypes is returned when an image definition has no types defined.
	ErrNoTypes = errors.New("no types defined")

	// ErrMissingBase is returned when a type template has no base image.
	ErrMissingBase = errors.New("missing required field: base")

	// ErrMissingFIPSProfile is returned when types.fips has no fips-profile.
	ErrMissingFIPSProfile = errors.New("missing required field: fips-profile")

	// ErrInvalidFIPSProfile is returned when fips-profile is not supported.
	ErrInvalidFIPSProfile = errors.New("invalid fips-profile")

	// ErrFIPSProfileOnNonFIPS is returned when a non-fips type declares fips-profile.
	ErrFIPSProfileOnNonFIPS = errors.New("fips-profile is only valid on type fips")

	// ErrInvalidFIPSBase is returned when a profile uses an incompatible base.
	ErrInvalidFIPSBase = errors.New("invalid fips base")

	// ErrMissingFIPSEnvironment is returned when a profile lacks its runtime toggle.
	ErrMissingFIPSEnvironment = errors.New("missing fips environment")

	// ErrMelangeSourceConflict is returned when both upstream and bespoke are set.
	ErrMelangeSourceConflict = errors.New("melange: set exactly one of upstream or bespoke, not both")

	// ErrMelangeNoSource is returned when neither upstream nor bespoke is set.
	ErrMelangeNoSource = errors.New("melange: one of upstream or bespoke is required")

	// ErrMelangePathTraversal is returned when a filename field contains a path separator or traversal sequence.
	ErrMelangePathTraversal = errors.New("melange: filename fields must not contain path separators or traversal sequences")
)

// LoadConfig loads the global integer.yaml configuration file.
func LoadConfig(path string) (*IntegerConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config %q: %w", path, err)
	}
	var cfg IntegerConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config %q: %w", path, err)
	}
	return &cfg, nil
}

// LoadImage loads an images/<name>.yaml image definition file.
func LoadImage(path string) (*ImageDef, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading image %q: %w", path, err)
	}
	var def ImageDef
	if err := yaml.Unmarshal(data, &def); err != nil {
		return nil, fmt.Errorf("parsing image %q: %w", path, err)
	}
	return &def, nil
}

// Validate returns a non-nil error if the ImageDef is missing required fields.
func Validate(def *ImageDef) error {
	if def.Name == "" {
		return ErrMissingName
	}
	if def.Upstream.Package == "" {
		return fmt.Errorf("image %q: %w", def.Name, ErrMissingPackage)
	}
	if len(def.Types) == 0 {
		return fmt.Errorf("image %q: %w", def.Name, ErrNoTypes)
	}
	for typeName, tmpl := range def.Types {
		if tmpl.Base == "" {
			return fmt.Errorf("image %q type %q: %w", def.Name, typeName, ErrMissingBase)
		}
		if err := validateFIPSProfile(def.Name, typeName, tmpl); err != nil {
			return err
		}
		if err := validateMelange(def.Name, typeName, tmpl.Melange); err != nil {
			return err
		}
	}
	return nil
}

func validateFIPSProfile(image, typeName string, tmpl TypeTemplate) error {
	if typeName != "fips" {
		if tmpl.FIPSProfile != "" {
			return fmt.Errorf("image %q type %q: %w", image, typeName, ErrFIPSProfileOnNonFIPS)
		}
		return nil
	}
	if tmpl.FIPSProfile == "" {
		return fmt.Errorf("image %q type %q: %w", image, typeName, ErrMissingFIPSProfile)
	}
	if !tmpl.FIPSProfile.Valid() {
		return fmt.Errorf("image %q type %q value %q: %w", image, typeName, tmpl.FIPSProfile, ErrInvalidFIPSProfile)
	}
	switch tmpl.FIPSProfile {
	case FIPSProfileGo:
		if !envContains(tmpl, "GODEBUG", "fips140=on") {
			return fmt.Errorf("image %q type %q profile %q: %w", image, typeName, tmpl.FIPSProfile, ErrMissingFIPSEnvironment)
		}
	case FIPSProfileOpenSSL:
		if tmpl.Base != "wolfi-fips" {
			return fmt.Errorf("image %q type %q profile %q base %q: %w", image, typeName, tmpl.FIPSProfile, tmpl.Base, ErrInvalidFIPSBase)
		}
	case FIPSProfileJava:
		if tmpl.Base != "wolfi-fips" {
			return fmt.Errorf("image %q type %q profile %q base %q: %w", image, typeName, tmpl.FIPSProfile, tmpl.Base, ErrInvalidFIPSBase)
		}
		if !envContains(tmpl, "JAVA_TOOL_OPTIONS", "java.security.properties") {
			return fmt.Errorf("image %q type %q profile %q: %w", image, typeName, tmpl.FIPSProfile, ErrMissingFIPSEnvironment)
		}
	case FIPSProfileReview:
		return nil
	}
	return nil
}

func envContains(tmpl TypeTemplate, key, token string) bool {
	return strings.Contains(tmpl.Environment[key], token)
}

func validateMelange(image, typeName string, m *MelangeSpec) error {
	if m == nil {
		return nil
	}
	if m.Upstream != "" && m.Bespoke != "" {
		return fmt.Errorf("image %q type %q: %w", image, typeName, ErrMelangeSourceConflict)
	}
	if m.Upstream == "" && m.Bespoke == "" {
		return fmt.Errorf("image %q type %q: %w", image, typeName, ErrMelangeNoSource)
	}
	if err := validateFilename(image, typeName, "bespoke", m.Bespoke); err != nil {
		return err
	}
	if err := validateFilename(image, typeName, "env-file", m.EnvFile); err != nil {
		return err
	}
	return nil
}

func validateFilename(image, typeName, field, value string) error {
	if value == "" {
		return nil
	}
	if strings.Contains(value, "/") || strings.Contains(value, `\`) || strings.Contains(value, "..") {
		return fmt.Errorf("image %q type %q field %q: %w", image, typeName, field, ErrMelangePathTraversal)
	}
	return nil
}
