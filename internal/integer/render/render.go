// Package render converts ImageDef TypeTemplates into apko-compatible YAML.
package render

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/verity-org/verity/internal/integer/config"
)

const placeholder = "{{version}}"

// apkoConfig is the YAML structure written for apko. Only fields used by
// integer are represented here; apko ignores unknown fields.
type apkoConfig struct {
	Include    string            `yaml:"include,omitempty"`
	Contents   *apkoContents     `yaml:"contents,omitempty"`
	Entrypoint *apkoEntrypoint   `yaml:"entrypoint,omitempty"`
	WorkDir    string            `yaml:"work-dir,omitempty"`
	Environ    map[string]string `yaml:"environment,omitempty"`
	Paths      []apkoPath        `yaml:"paths,omitempty"`
}

// apkoContents holds the package list section.
type apkoContents struct {
	Packages []string `yaml:"packages"`
}

// apkoEntrypoint holds the entrypoint section.
type apkoEntrypoint struct {
	Command string `yaml:"command"`
}

// apkoPath is one path entry in the apko config.
// Permissions is uint32 so apko's YAML parser receives an integer (e.g. 493),
// not a string like "0o755" which would fail to unmarshal into apko's uint32 field.
type apkoPath struct {
	Path        string `yaml:"path"`
	Type        string `yaml:"type,omitempty"`
	UID         int    `yaml:"uid"`
	GID         int    `yaml:"gid"`
	Permissions uint32 `yaml:"permissions,omitempty"`
}

// Config renders an apko YAML config from a TypeTemplate for a specific version.
//
// basePath is the path to the _base/ directory relative to where the generated
// file will be written (e.g. "../../../_base"). The rendered include: directive
// will be "<basePath>/<tmpl.Base>.yaml".
//
// version is substituted for every occurrence of "{{version}}" in all string
// fields. Pass an empty string for unversioned images.
func Config(tmpl *config.TypeTemplate, version, basePath string) ([]byte, error) {
	cfg := apkoConfig{}

	// Include directive pointing to the base file.
	cfg.Include = filepath.Join(basePath, tmpl.Base+".yaml")

	// Package list with version substitution.
	if len(tmpl.Packages) > 0 {
		pkgs := make([]string, len(tmpl.Packages))
		for i, p := range tmpl.Packages {
			pkgs[i] = sub(p, version)
		}
		cfg.Contents = &apkoContents{Packages: pkgs}
	}

	// Entrypoint.
	if tmpl.Entrypoint != "" {
		cfg.Entrypoint = &apkoEntrypoint{Command: sub(tmpl.Entrypoint, version)}
	}

	// Working directory.
	if tmpl.WorkDir != "" {
		cfg.WorkDir = sub(tmpl.WorkDir, version)
	}

	// Environment variables.
	if len(tmpl.Environment) > 0 {
		cfg.Environ = make(map[string]string, len(tmpl.Environment))
		for k, v := range tmpl.Environment {
			cfg.Environ[sub(k, version)] = sub(v, version)
		}
	}

	// Paths.
	if len(tmpl.Paths) > 0 {
		cfg.Paths = make([]apkoPath, len(tmpl.Paths))
		for i, p := range tmpl.Paths {
			ptype := p.Type
			if ptype == "" {
				ptype = "directory"
			}
			perms, err := parsePermissions(p.Permissions)
			if err != nil {
				return nil, fmt.Errorf("path %q: %w", p.Path, err)
			}
			cfg.Paths[i] = apkoPath{
				Path:        sub(p.Path, version),
				Type:        ptype,
				UID:         p.UID,
				GID:         p.GID,
				Permissions: perms,
			}
		}
	}

	out, err := yaml.Marshal(&cfg)
	if err != nil {
		return nil, fmt.Errorf("marshalling apko config: %w", err)
	}
	return out, nil
}

// sub replaces all occurrences of {{version}} in s with version.
func sub(s, version string) string {
	if version == "" {
		return s
	}
	return strings.ReplaceAll(s, placeholder, version)
}

// parsePermissions converts a permissions string (e.g. "0o755", "0755", "755")
// into a uint32. The input is treated as octal.
func parsePermissions(s string) (uint32, error) {
	if s == "" {
		return 0, nil
	}
	// Strip Go-style octal prefix "0o" if present.
	octal := strings.TrimPrefix(s, "0o")
	n, err := strconv.ParseUint(octal, 8, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid permissions %q (expected octal like 0o755): %w", s, err)
	}
	return uint32(n), nil
}
