package config

import (
	"errors"
	"fmt"
	"os"
	"slices"
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

	// ErrFIPSProfileOnNonFIPS is returned when a non-fips-claiming type declares fips-profile.
	ErrFIPSProfileOnNonFIPS = errors.New("fips-profile is only valid on fips-claiming types")

	// ErrInvalidFIPSBase is returned when a profile uses an incompatible base.
	ErrInvalidFIPSBase = errors.New("invalid fips base")

	// ErrMissingFIPSEnvironment is returned when a profile lacks its runtime toggle.
	ErrMissingFIPSEnvironment = errors.New("missing fips environment")

	// ErrUnsupportedFIPSProfile is returned when a profile lacks a provider artifact.
	ErrUnsupportedFIPSProfile = errors.New("unsupported fips-profile")

	// ErrMelangeSourceConflict is returned when both upstream and bespoke are set.
	ErrMelangeSourceConflict = errors.New("melange: set exactly one of upstream or bespoke, not both")

	// ErrMelangeNoSource is returned when neither upstream nor bespoke is set.
	ErrMelangeNoSource = errors.New("melange: one of upstream or bespoke is required")

	// ErrMelangePathTraversal is returned when a filename field contains a path separator or traversal sequence.
	ErrMelangePathTraversal = errors.New("melange: filename fields must not contain path separators or traversal sequences")

	// ErrMelangeTypeNotFound is returned when a version scopes Melange configuration to an undefined image type.
	ErrMelangeTypeNotFound = errors.New("melange: version-scoped type is not defined")

	// ErrInvalidMelangeVersion is returned when a version-scoped Melange key is unsafe for placeholder substitution.
	ErrInvalidMelangeVersion = errors.New("melange: invalid version scope")
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
	return parseImage(data, path)
}

func parseImage(data []byte, path string) (*ImageDef, error) {
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
	for typeName := range def.Types {
		tmpl := def.Types[typeName]
		if tmpl.Base == "" {
			return fmt.Errorf("image %q type %q: %w", def.Name, typeName, ErrMissingBase)
		}
		if err := validateMelange(def.Name, typeName, tmpl.Melange); err != nil {
			return err
		}
		if err := validateFIPSProfile(def.Name, typeName, &tmpl); err != nil {
			return err
		}
	}
	for version, meta := range def.Versions {
		if len(meta.Melange) > 0 && !ValidMelangeVersion(version) {
			return fmt.Errorf("image %q version %q: %w", def.Name, version, ErrInvalidMelangeVersion)
		}
		for typeName, melangeSpec := range meta.Melange {
			tmpl, ok := def.Types[typeName]
			if !ok {
				return fmt.Errorf("image %q version %q type %q: %w", def.Name, version, typeName, ErrMelangeTypeNotFound)
			}
			if err := validateMelange(def.Name, typeName, melangeSpec); err != nil {
				return fmt.Errorf("image %q version %q: %w", def.Name, version, err)
			}
			tmpl.Melange = melangeSpec
			if err := validateFIPSProfile(def.Name, typeName, &tmpl); err != nil {
				return fmt.Errorf("image %q version %q: %w", def.Name, version, err)
			}
		}
	}
	return nil
}

func validateFIPSProfile(image, typeName string, tmpl *TypeTemplate) error {
	if !isFIPSClaimType(typeName) {
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
		return validateGoFIPS(image, typeName, tmpl)
	case FIPSProfileOpenSSL:
		return validateOpenSSLFIPS(image, typeName, tmpl)
	case FIPSProfileJava:
		return validateProviderFIPS(image, typeName, tmpl)
	case FIPSProfileReview:
		return nil
	}
	return nil
}

// validateGoFIPS enforces the Go Cryptographic Module path: the wolfi-base
// runtime, GODEBUG=fips140=on, and a pinned validated module (GOFIPS140=v1.0.0
// directly or via the fips.env melange override).
func validateGoFIPS(image, typeName string, tmpl *TypeTemplate) error {
	if tmpl.Base != "wolfi-base" {
		return fmt.Errorf("image %q type %q profile %q base %q: %w", image, typeName, tmpl.FIPSProfile, tmpl.Base, ErrInvalidFIPSBase)
	}
	if !envContains(tmpl, "GODEBUG", "fips140=on") {
		return fmt.Errorf("image %q type %q profile %q: %w", image, typeName, tmpl.FIPSProfile, ErrMissingFIPSEnvironment)
	}
	if tmpl.Environment["GOFIPS140"] != "v1.0.0" && !usesFIPSEnvFile(tmpl) {
		return fmt.Errorf("image %q type %q profile %q: %w", image, typeName, tmpl.FIPSProfile, ErrMissingFIPSEnvironment)
	}
	return nil
}

func validateOpenSSLFIPS(image, typeName string, tmpl *TypeTemplate) error {
	if tmpl.Base != "wolfi-fips" {
		return fmt.Errorf("image %q type %q profile %q base %q: %w", image, typeName, tmpl.FIPSProfile, tmpl.Base, ErrInvalidFIPSBase)
	}
	if !hasPackage(tmpl, "openssl-provider-fips") || tmpl.Melange == nil || !tmpl.Melange.Bespoke.Contains("openssl-provider-fips.yaml") {
		return fmt.Errorf("image %q type %q profile %q: %w", image, typeName, tmpl.FIPSProfile, ErrUnsupportedFIPSProfile)
	}
	command, ok := strings.CutPrefix(tmpl.Entrypoint, "/usr/bin/openssl-fips-entrypoint ")
	if !ok || strings.TrimSpace(command) == "" {
		return fmt.Errorf("image %q type %q profile %q: %w", image, typeName, tmpl.FIPSProfile, ErrMissingFIPSEnvironment)
	}
	if tmpl.Environment["OPENSSL_MODULES"] != "/usr/lib/ossl-modules" || tmpl.Environment["OPENSSL_CONF"] != "/etc/ssl/openssl-fips.cnf" {
		return fmt.Errorf("image %q type %q profile %q: %w", image, typeName, tmpl.FIPSProfile, ErrMissingFIPSEnvironment)
	}
	return nil
}

func hasPackage(tmpl *TypeTemplate, name string) bool {
	return slices.Contains(tmpl.Packages, name)
}

func validateProviderFIPS(image, typeName string, tmpl *TypeTemplate) error {
	if tmpl.Base != "wolfi-fips" {
		return fmt.Errorf("image %q type %q profile %q base %q: %w", image, typeName, tmpl.FIPSProfile, tmpl.Base, ErrInvalidFIPSBase)
	}
	return fmt.Errorf("image %q type %q profile %q: %w", image, typeName, tmpl.FIPSProfile, ErrUnsupportedFIPSProfile)
}

func isFIPSClaimType(typeName string) bool {
	return typeName == "fips" || strings.HasPrefix(typeName, "fips-") || strings.HasSuffix(typeName, "-fips")
}

func envContains(tmpl *TypeTemplate, key, token string) bool {
	return strings.Contains(tmpl.Environment[key], token)
}

func usesFIPSEnvFile(tmpl *TypeTemplate) bool {
	return tmpl.Melange != nil && tmpl.Melange.EnvFile == "fips.env"
}

func validateMelange(image, typeName string, m *MelangeSpec) error {
	if m == nil {
		return nil
	}
	if m.Upstream != "" && len(m.Bespoke) > 0 {
		return fmt.Errorf("image %q type %q: %w", image, typeName, ErrMelangeSourceConflict)
	}
	if m.Upstream == "" && len(m.Bespoke) == 0 {
		return fmt.Errorf("image %q type %q: %w", image, typeName, ErrMelangeNoSource)
	}
	for _, bespoke := range m.Bespoke {
		if err := validateFilename(image, typeName, "bespoke", bespoke); err != nil {
			return err
		}
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
