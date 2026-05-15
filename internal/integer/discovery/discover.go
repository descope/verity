// Package discovery walks the images/ directory, resolves available versions
// from the Wolfi APKINDEX, renders apko config templates, and returns a flat
// list of all buildable name × version × type combinations.
package discovery

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/verity-org/verity/internal/integer/apkindex"
	"github.com/verity-org/verity/internal/integer/config"
	"github.com/verity-org/verity/internal/integer/render"
)

// latestSentinel is the special version string used for unversioned images.
const latestSentinel = "latest"

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

// ResolveStreamRenderVersion returns the version stem that render.Config
// must substitute for every "{{version}}" placeholder in the apko
// template — `packages:` constraints AND any other string field that
// uses the placeholder. For "1.21" / "22" streams that map directly to
// a Wolfi APK, the returned stem equals streamVersion. For floating-
// major streams whose literal name doesn't exist in Wolfi (e.g.
// "kyverno-1" → only "kyverno-1.17" is published), the returned stem is
// the highest matching minor ("1.17") so apko's apk solver can satisfy
// the constraint.
//
// The resolution pattern (the package template that drives apk
// solving) is picked by ImageDef.VersionedPackagePattern: for most
// images it is upstream.package (kyverno, cilium, crossplane,
// fluent-bit, prometheus, istio-*, …); for the erlang/haproxy shape
// where upstream.package is unversioned, it is the first
// "{{version}}"-templated entry across types[*].packages. When neither
// has a placeholder, upstream.package is used as a fallback (no alias
// resolution applies in that case).
//
// pkgs == nil OR len(pkgs) == 0 disables aliasing (returns
// streamVersion unchanged) so that offline builds remain
// deterministic; the caller is responsible for surfacing
// unresolvable cases via `verity integer validate`. The
// empty-but-non-nil case matches `apkindex.ResolveAliasVersion`'s
// own short-circuit, so callers can pass either shape interchangeably.
//
// This helper centralises the alias logic so the discovery path
// (expandImage) and the local CLI build path (cmd/integer build) stay
// in lockstep — see the PR #307 follow-up that exposed the gap.
func ResolveStreamRenderVersion(def *config.ImageDef, pkgs []apkindex.Package, streamVersion string) string {
	if def == nil {
		// Both call sites (discovery.expandImage and cmd.runIntegerBuild)
		// validate def upstream of this helper, but the helper is exported
		// and could be called by future callers without that guarantee.
		// Returning streamVersion unchanged is the safe no-op: the caller
		// then renders the literal as if no alias resolution happened,
		// which matches the offline / no-APKINDEX behavior.
		return streamVersion
	}
	resolutionPattern := def.VersionedPackagePattern()
	if resolutionPattern == "" {
		resolutionPattern = def.Upstream.Package
	}
	return apkindex.ResolveAliasVersion(pkgs, resolutionPattern, streamVersion)
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

	// resolutionPattern is the same for every version of this image; compute
	// once and reuse for the per-version full-version lookup below.
	resolutionPattern := def.VersionedPackagePattern()
	if resolutionPattern == "" {
		resolutionPattern = def.Upstream.Package
	}

	var results []DiscoveredImage

	for _, v := range versions {
		renderVersion := ResolveStreamRenderVersion(def, pkgs, v)

		// Resolve full version from APKINDEX for semver tag expansion. Use
		// the aliased stem so semver cascade tags ("1.17", "1.17.5") still
		// expand correctly when the declared stream is a floating-major.
		fullVersion := apkindex.ResolveFullVersion(pkgs, resolutionPattern, renderVersion)
		tags := DeriveTags(v, latestVersion, fullVersion)

		for typeName := range def.Types {
			if ShouldSkipType(def, v, typeName) {
				continue
			}
			tmpl := def.Types[typeName]

			out, err := render.Config(&tmpl, renderVersion, basePath)
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

// ShouldSkipType reports whether a type should be omitted for a specific
// version. A type is skipped when:
//   - The version's skip-types list includes the type name, OR
//   - The version is auto-discovered (not in the versions map) and the type
//     requires a melange build. Melange upstream YAMLs target a specific
//     version, so auto-discovered versions cannot use them.
func ShouldSkipType(def *config.ImageDef, version, typeName string) bool {
	meta, ok := def.Versions[version]
	if !ok {
		// Auto-discovered version: skip types that require a melange build.
		tmpl, exists := def.Types[typeName]
		return exists && tmpl.Melange != nil
	}
	return slices.Contains(meta.SkipTypes, typeName)
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
	// Drop the auto-discovered "latest" sentinel when explicit non-"latest"
	// versions exist — the explicit streams already handle all tags including
	// "latest". Keeps explicitly declared "latest" (in def.Versions) intact.
	if seen[latestSentinel] {
		if _, explicit := def.Versions[latestSentinel]; !explicit {
			for v := range seen {
				if v != latestSentinel {
					delete(seen, latestSentinel)
					break
				}
			}
		}
	}

	versions := make([]string, 0, len(seen))
	for v := range seen {
		versions = append(versions, v)
	}
	apkindex.SortVersions(versions)
	return versions
}

// DeriveTags returns the base tags for a version, including semver cascade
// tags derived from the full APKINDEX version. The fullVersion parameter is
// the stripped package version (e.g. "22.16.0"); pass "" to skip expansion.
//
// For normal streams (e.g. "22"), cascade tags more specific than the stream
// are added: "22" + "22.16.0" → ["22", "22.16", "22.16.0"].
// For the "latest" stream, all cascade tags are added.
// The latestVersion carries the "latest" tag.
func DeriveTags(streamVersion, latestVersion, fullVersion string) []string {
	if streamVersion == latestSentinel {
		tags := make([]string, 1, 1+len(SemverCascade(fullVersion)))
		tags[0] = latestSentinel
		tags = append(tags, SemverCascade(fullVersion)...)
		return tags
	}
	tags := []string{streamVersion}
	for _, ct := range SemverCascade(fullVersion) {
		if strings.HasPrefix(ct, streamVersion+".") {
			tags = append(tags, ct)
		}
	}
	if streamVersion == latestVersion {
		tags = append(tags, latestSentinel)
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
	// Backward iteration via slices.Backward so the modernize linter
	// is satisfied while keeping the highest-numeric-first walk
	// semantics (highest version is at the end of the sorted slice).
	for _, v := range slices.Backward(versions) {
		if v != latestSentinel {
			return v
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

// SemverCascade splits a dot-separated version into all prefix combinations,
// shortest first. "22.16.0" → ["22", "22.16", "22.16.0"].
// Returns nil for empty input.
func SemverCascade(fullVersion string) []string {
	if fullVersion == "" {
		return nil
	}
	parts := strings.Split(fullVersion, ".")
	result := make([]string, len(parts))
	for i := range parts {
		result[i] = strings.Join(parts[:i+1], ".")
	}
	return result
}
