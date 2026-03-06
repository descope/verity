package config

import (
	"errors"
	"fmt"
	"os"

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
	}
	return nil
}
