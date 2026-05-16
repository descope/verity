// Package render converts ImageDef TypeTemplates into apko-compatible YAML.
package render

import (
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/verity-org/verity/internal/integer/config"
)

const placeholder = "{{version}}"

// Sentinel errors returned by Config when path entries violate the
// type/source invariants enforced for apko compatibility.
var (
	// ErrSymlinkRequiresSource is returned when a path entry has
	// type=symlink but no Source. apko rejects symlink entries without
	// a target at melange/apko build time; we fail fast at render.
	ErrSymlinkRequiresSource = errors.New("type=symlink requires source")
	// ErrSourceOnNonSymlink is returned when a path entry sets Source
	// without type=symlink. apko rejects source on directory (and other)
	// types; we fail fast at render.
	ErrSourceOnNonSymlink = errors.New("source is only valid for type=symlink")
)

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
	Source      string `yaml:"source,omitempty"`
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
		paths, err := convertPaths(tmpl.Paths, version)
		if err != nil {
			return nil, err
		}
		cfg.Paths = paths
	}

	out, err := yaml.Marshal(&cfg)
	if err != nil {
		return nil, fmt.Errorf("marshalling apko config: %w", err)
	}
	return out, nil
}

// convertPaths transforms a slice of config.PathDef entries into the apko
// representation, applying {{version}} substitution and enforcing the
// source/type invariants apko itself enforces at build time:
//
//   - type=symlink requires Source (apko rejects symlinks without a target)
//   - Source set on any non-symlink type is invalid (apko rejects it)
//
// It also REORDERS entries to defend against an apko quirk: apko's
// mutatePaths (chainguard-dev/apko pkg/build/paths.go:146) runs
// mutatePermissions on every non-permissions mutation AFTER its type-
// specific mutator, using the mutation's `permissions:` field. A symlink
// mutation with no explicit `permissions:` defaults to 0, so apko ends up
// calling Chmod(0) on the SYMLINK PATH — and under Linux symlink-chmod
// semantics that follows the link and resets the TARGET file's mode.
// Concretely: if `/usr/bin/etcd` has a `permissions: 0o755` mutation and
// `/usr/local/bin/etcd → /usr/bin/etcd` is mutated AFTER it, the symlink's
// implicit Chmod(0) wipes the binary's executable bit and `runc exec`
// aborts with permission denied at chart-integration time.
//
// To make this safe regardless of YAML ordering in images/*.yaml, we
// reorder so every `symlink` entry comes BEFORE every `permissions`
// entry whose `path:` is the symlink's `source:` (the link target). The
// reorder is stable within each group — relative ordering of entries
// that don't trigger the rule is preserved.
//
// Failing fast at render keeps misconfigured YAML errors close to their YAML
// source instead of surfacing as opaque apko/melange build failures.
func convertPaths(in []config.PathDef, version string) ([]apkoPath, error) {
	out := make([]apkoPath, len(in))
	for i, p := range in {
		ptype := p.Type
		if ptype == "" {
			ptype = "directory"
		}
		if ptype == "symlink" && p.Source == "" {
			return nil, fmt.Errorf("path %q: %w", p.Path, ErrSymlinkRequiresSource)
		}
		if ptype != "symlink" && p.Source != "" {
			return nil, fmt.Errorf("path %q (got type=%q): %w", p.Path, ptype, ErrSourceOnNonSymlink)
		}
		perms, err := parsePermissions(p.Permissions)
		if err != nil {
			return nil, fmt.Errorf("path %q: %w", p.Path, err)
		}
		out[i] = apkoPath{
			Path:        sub(p.Path, version),
			Type:        ptype,
			Source:      sub(p.Source, version),
			UID:         p.UID,
			GID:         p.GID,
			Permissions: perms,
		}
	}
	return reorderForApkoChmodQuirk(out), nil
}

// reorderForApkoChmodQuirk moves every `symlink` entry to come BEFORE every
// `permissions` entry whose path equals the symlink's source. See the
// convertPaths docstring for the underlying apko behaviour. Pure-data
// reorder; no semantic change beyond mutation order.
func reorderForApkoChmodQuirk(in []apkoPath) []apkoPath {
	// Build a set of "symlink sources that will collide with permissions
	// entries already past this point" so we know which symlinks to lift.
	permsBefore := make(map[string]struct{})
	for _, p := range in {
		if p.Type == "permissions" {
			permsBefore[p.Path] = struct{}{}
		}
	}
	// Two-pass stable rebuild: first emit symlinks whose source matches
	// a permissions entry's path (in their original relative order),
	// then emit everything else (including any unrelated symlinks)
	// in original order. This is the smallest reorder that satisfies
	// the apko quirk.
	out := make([]apkoPath, 0, len(in))
	for _, p := range in {
		if p.Type == "symlink" {
			if _, hit := permsBefore[p.Source]; hit {
				out = append(out, p)
			}
		}
	}
	for _, p := range in {
		if p.Type == "symlink" {
			if _, hit := permsBefore[p.Source]; hit {
				continue
			}
		}
		out = append(out, p)
	}
	return out
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
