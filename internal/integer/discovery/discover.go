// Package discovery walks the images/ directory, resolves available versions
// from the Wolfi APKINDEX, renders apko config templates, and returns a flat
// list of all buildable name × version × type combinations.
package discovery

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/verity-org/verity/internal/integer/apkindex"
	"github.com/verity-org/verity/internal/integer/config"
	"github.com/verity-org/verity/internal/integer/render"
)

// DiscoveredImage represents one buildable image: a name × version × type.
type DiscoveredImage struct {
	Name     string   `json:"name"`
	Version  string   `json:"version"`
	Type     string   `json:"type"`
	File     string   `json:"file"` // absolute path to the generated apko YAML
	Tags     []string `json:"tags"`
	Registry string   `json:"registry"`
}

// Options configures the Discover call.
type Options struct {
	ImagesDir string
	Registry  string
	// Packages is the parsed APKINDEX. If nil, only versions declared in the
	// image file's versions map are built (no auto-discovery).
	Packages []apkindex.Package
	// GenDir is the directory where generated apko YAML files are written.
	// Defaults to a system temp directory if empty.
	GenDir string
}

// DiscoverFromFiles walks imagesDir for *.yaml files (not subdirectories),
// resolves versions from APKINDEX, and returns every buildable combination.
// This is the primary entry point for the v2 flat-file layout.
func DiscoverFromFiles(opts Options) ([]DiscoveredImage, error) {
	entries, err := os.ReadDir(opts.ImagesDir)
	if err != nil {
		return nil, fmt.Errorf("reading images dir %q: %w", opts.ImagesDir, err)
	}

	genDir := opts.GenDir
	if genDir == "" {
		var tmpErr error
		genDir, tmpErr = os.MkdirTemp("", "integer-gen-*")
		if tmpErr != nil {
			return nil, fmt.Errorf("creating temp dir: %w", tmpErr)
		}
	}

	var results []DiscoveredImage

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}

		defPath := filepath.Join(opts.ImagesDir, entry.Name())
		def, err := config.LoadImage(defPath)
		if err != nil {
			return nil, fmt.Errorf("loading %q: %w", entry.Name(), err)
		}
		if err := config.Validate(def); err != nil {
			return nil, fmt.Errorf("invalid image %q: %w", entry.Name(), err)
		}

		imgs, err := expandImage(def, opts.ImagesDir, opts.Registry, opts.Packages, genDir)
		if err != nil {
			return nil, fmt.Errorf("expanding image %q: %w", def.Name, err)
		}
		results = append(results, imgs...)
	}

	return results, nil
}

// expandImage converts one ImageDef into DiscoveredImage entries by
// resolving versions and rendering apko configs for each version × type.
func expandImage(def *config.ImageDef, imagesDir, registry string, pkgs []apkindex.Package, genDir string) ([]DiscoveredImage, error) {
	versions := ResolveVersions(def, pkgs)
	if len(versions) == 0 {
		return nil, nil
	}

	basePath := filepath.Join(imagesDir, "_base")
	latestVersion := FindLatestVersion(versions)

	var results []DiscoveredImage

	for _, v := range versions {
		tags := DeriveTags(v, latestVersion)
		for typeName := range def.Types {
			tmpl := def.Types[typeName]

			out, err := render.Config(&tmpl, v, basePath)
			if err != nil {
				return nil, fmt.Errorf("rendering config for %s:%s-%s: %w", def.Name, v, typeName, err)
			}

			genFile := filepath.Join(genDir, def.Name, v, typeName+".apko.yaml")
			if err := os.MkdirAll(filepath.Dir(genFile), 0o755); err != nil {
				return nil, fmt.Errorf("creating gen dir: %w", err)
			}
			if err := os.WriteFile(genFile, out, 0o644); err != nil {
				return nil, fmt.Errorf("writing gen file: %w", err)
			}

			typeTags := ApplyTypeSuffix(tags, typeName)
			results = append(results, DiscoveredImage{
				Name:     def.Name,
				Version:  v,
				Type:     typeName,
				File:     genFile,
				Tags:     typeTags,
				Registry: registry,
			})
		}
	}

	// Sort for deterministic output: numeric-aware version order, then type.
	sort.Slice(results, func(i, j int) bool {
		if results[i].Version != results[j].Version {
			return apkindex.VersionLess(results[i].Version, results[j].Version)
		}
		return results[i].Type < results[j].Type
	})

	return results, nil
}

// ResolveVersions merges auto-discovered APKINDEX versions with the
// human-curated versions map. Returns a sorted slice of version strings.
func ResolveVersions(def *config.ImageDef, pkgs []apkindex.Package) []string {
	seen := make(map[string]bool)

	// Auto-discover from APKINDEX.
	if len(pkgs) > 0 {
		for _, v := range apkindex.DiscoverVersions(pkgs, def.Upstream.Package) {
			seen[v] = true
		}
	}

	// Always include explicitly declared versions (even if not in APKINDEX).
	for v := range def.Versions {
		seen[v] = true
	}

	versions := make([]string, 0, len(seen))
	for v := range seen {
		versions = append(versions, v)
	}
	apkindex.SortVersions(versions)
	return versions
}

// DeriveTags returns the base tags for a version. The latestVersion parameter
// specifies which version should carry the "latest" tag (computed dynamically
// as the highest version by the caller).
func DeriveTags(version, latestVersion string) []string {
	if version == "latest" {
		return []string{"latest"}
	}
	tags := []string{version}
	if version == latestVersion {
		tags = append(tags, "latest")
	}
	return tags
}

// FindLatestVersion returns the highest numeric version from a sorted slice.
// The literal "latest" sentinel (used by unversioned packages) is skipped when
// numeric versions are present. Returns empty string if the slice is empty.
func FindLatestVersion(versions []string) string {
	if len(versions) == 0 {
		return ""
	}
	for i := len(versions) - 1; i >= 0; i-- {
		if versions[i] != "latest" {
			return versions[i]
		}
	}
	return versions[len(versions)-1]
}

// ApplyTypeSuffix appends "-<type>" to each tag for non-default types.
func ApplyTypeSuffix(tags []string, typeName string) []string {
	if typeName == "default" {
		result := make([]string, len(tags))
		copy(result, tags)
		return result
	}
	result := make([]string, len(tags))
	for i, t := range tags {
		result[i] = t + "-" + typeName
	}
	return result
}
