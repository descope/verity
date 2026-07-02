package apkindex

import (
	"sort"
	"strings"
)

const versionPlaceholder = "{{version}}"

// DiscoverVersions returns the version stems available in pkgs for the given
// package pattern. The pattern may contain "{{version}}" as a placeholder.
//
// Versioned pattern (contains "{{version}}"):
//
//	Pattern "nodejs-{{version}}" matches nodejs-20, nodejs-22, nodejs-24 and
//	returns ["20", "22", "24"] (unique, sorted lexicographically).
//
// Unversioned pattern (no placeholder):
//
//	Pattern "curl" returns ["latest"] if the package exists, or an empty slice
//	if it does not.
func DiscoverVersions(pkgs []Package, pattern string) []string {
	if !strings.Contains(pattern, versionPlaceholder) {
		return discoverUnversioned(pkgs, pattern)
	}
	return discoverVersioned(pkgs, pattern)
}

// SortVersions sorts a slice of version strings using numeric-aware ordering.
// "1.10" > "1.9" and "22" > "20". Modifies the slice in place.
func SortVersions(versions []string) {
	sort.Slice(versions, func(i, j int) bool {
		return versionLess(versions[i], versions[j])
	})
}

// discoverVersioned extracts unique version stems from package names matching
// the pattern. E.g. "nodejs-{{version}}" extracts "20", "22", "24" from the
// package names nodejs-20, nodejs-22, nodejs-24.
func discoverVersioned(pkgs []Package, pattern string) []string {
	before, after, _ := strings.Cut(pattern, versionPlaceholder)
	prefix := before
	suffix := after

	seen := make(map[string]bool)
	for _, pkg := range pkgs {
		name := pkg.Name
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		stem := strings.TrimPrefix(name, prefix)
		if suffix != "" && !strings.HasSuffix(stem, suffix) {
			continue
		}
		stem = strings.TrimSuffix(stem, suffix)
		if isVersionStem(stem) {
			seen[stem] = true
		}
	}

	versions := make([]string, 0, len(seen))
	for v := range seen {
		versions = append(versions, v)
	}
	SortVersions(versions)
	return versions
}

// discoverUnversioned returns ["latest"] if the exact package name exists.
func discoverUnversioned(pkgs []Package, name string) []string {
	for _, pkg := range pkgs {
		if pkg.Name == name {
			return []string{"latest"}
		}
	}
	return nil
}

// isVersionStem returns true if s looks like a pure version stem: non-empty,
// no hyphens, starts with a digit, and contains only digits and dots.
// This filters out sibling-package suffixes like "gateway" (envoy-gateway),
// "ratelimit" (envoy-ratelimit), or free-threaded markers like "3.14t".
func isVersionStem(s string) bool {
	if s == "" || s[0] < '0' || s[0] > '9' {
		return false
	}
	for _, c := range s {
		if c != '.' && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}

// VersionLess reports whether version a is less than b using numeric-aware
// comparison. "1.9" < "1.10" and "20" < "22".
func VersionLess(a, b string) bool {
	return versionLess(a, b)
}

// StartsNumeric reports whether v begins with an ASCII digit — the shape of
// a version stream ("22", "1.17") as opposed to a variant alias ("nonroot").
func StartsNumeric(v string) bool {
	return v != "" && v[0] >= '0' && v[0] <= '9'
}

// versionLess compares version stems lexicographically with numeric awareness.
// "1.10" > "1.9" and "22" > "20".
func versionLess(a, b string) bool {
	aParts := splitVersion(a)
	bParts := splitVersion(b)
	for i := 0; i < len(aParts) && i < len(bParts); i++ {
		if aParts[i] != bParts[i] {
			// Compare numerically if both parts are digits.
			ai, aOK := parseNum(aParts[i])
			bi, bOK := parseNum(bParts[i])
			if aOK && bOK {
				return ai < bi
			}
			return aParts[i] < bParts[i]
		}
	}
	return len(aParts) < len(bParts)
}

// splitVersion splits a version string on "." and "-" boundaries.
func splitVersion(v string) []string {
	return strings.FieldsFunc(v, func(r rune) bool {
		return r == '.' || r == '-'
	})
}

// parseNum attempts to parse a string as an integer.
func parseNum(s string) (int, bool) {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int(c-'0')
	}
	return n, s != ""
}

// stripRevision removes the Alpine -rN revision suffix from a version string.
// "22.16.0-r0" → "22.16.0", "2.11.2-r1" → "2.11.2", "1.24.3" → "1.24.3".
func stripRevision(version string) string {
	idx := strings.LastIndex(version, "-r")
	if idx < 0 {
		return version
	}
	suffix := version[idx+2:]
	for _, c := range suffix {
		if c < '0' || c > '9' {
			return version
		}
	}
	if suffix == "" {
		return version
	}
	return version[:idx]
}

// LookupFullVersion returns the full version string (stripped of -rN revision
// suffix) for a specific package name. When multiple entries exist for the same
// name, the highest version wins. Returns "" if not found.
func LookupFullVersion(pkgs []Package, packageName string) string {
	var best string
	for _, pkg := range pkgs {
		if pkg.Name != packageName {
			continue
		}
		v := stripRevision(pkg.Version)
		if best == "" || versionLess(best, v) {
			best = v
		}
	}
	return best
}

// ResolveFullVersion resolves the full semver for a given version stream.
// For versioned patterns (containing "{{version}}"), it replaces the placeholder
// with streamVersion and looks up the resulting package name.
// For unversioned patterns, it looks up the pattern directly (streamVersion is ignored).
// Returns "" if not found.
func ResolveFullVersion(pkgs []Package, upstreamPattern, streamVersion string) string {
	if strings.Contains(upstreamPattern, versionPlaceholder) {
		name := strings.ReplaceAll(upstreamPattern, versionPlaceholder, streamVersion)
		return LookupFullVersion(pkgs, name)
	}
	return LookupFullVersion(pkgs, upstreamPattern)
}

// ResolveAliasVersion resolves the actual version stem to substitute into apko
// package constraints when an image declares a floating-major version that
// Wolfi does not publish as a meta-package.
//
// Wolfi APKINDEX naming convention is uneven:
//
//   - For some packages (go, nodejs, postgresql) Wolfi ships a virtual
//     meta-package per minor: e.g. "go-1.24" exists alongside per-patch
//     packages. Declared "1.24" stem resolves directly.
//
//   - For other packages (kyverno, cilium, crossplane, erlang, fluent-bit,
//     prometheus, istio-*, …) Wolfi only ships a specific minor:
//     "kyverno-1.17" exists, but "kyverno-1" does NOT. A declared stem of
//     "1" produces an apko constraint "kyverno-1" that the apk solver
//     cannot satisfy at publish time, manifesting as
//     `nothing provides "kyverno-1"`.
//
// ResolveAliasVersion bridges the gap. For a versioned upstreamPattern
// (containing "{{version}}") and a declared streamVersion:
//
//  1. If the literal "<prefix><streamVersion><suffix>" name exists in pkgs,
//     return streamVersion unchanged — the simple case.
//  2. Otherwise, search pkgs for the highest stem of the form
//     "<streamVersion>.<rest>" matching the pattern, and return that fuller
//     stem. E.g. pattern "kyverno-{{version}}", streamVersion "1", with
//     pkgs containing "kyverno-1.17" returns "1.17".
//  3. If neither matches, return streamVersion unchanged. The caller is
//     responsible for surfacing this — `verity integer validate` flags it
//     at PR time, and apko publish would otherwise fail at build time
//     with `nothing provides "<pkg>-<streamVersion>"`.
//
// For unversioned patterns (no placeholder) ResolveAliasVersion returns
// streamVersion unchanged: the upstream package name doesn't depend on a
// version stem so there's nothing to alias.
//
// pkgs == nil disables aliasing (returns streamVersion unchanged) so that
// offline `verity integer discover` calls still produce deterministic apko
// configs, identical to today's behaviour.
func ResolveAliasVersion(pkgs []Package, upstreamPattern, streamVersion string) string {
	if streamVersion == "" || len(pkgs) == 0 {
		return streamVersion
	}
	if !strings.Contains(upstreamPattern, versionPlaceholder) {
		return streamVersion
	}

	// 1) Literal match — preferred path; covers normal "1.21"-style streams.
	literal := strings.ReplaceAll(upstreamPattern, versionPlaceholder, streamVersion)
	for _, pkg := range pkgs {
		if pkg.Name == literal {
			return streamVersion
		}
	}

	// 2) Floating-major alias — search for "<streamVersion>.*" stems.
	before, after, _ := strings.Cut(upstreamPattern, versionPlaceholder)
	prefix := before
	suffix := after
	streamPrefix := streamVersion + "."

	var best string
	for _, pkg := range pkgs {
		name := pkg.Name
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		stem := strings.TrimPrefix(name, prefix)
		if suffix != "" && !strings.HasSuffix(stem, suffix) {
			continue
		}
		stem = strings.TrimSuffix(stem, suffix)
		if !strings.HasPrefix(stem, streamPrefix) {
			continue
		}
		if !isVersionStem(stem) {
			continue
		}
		if best == "" || versionLess(best, stem) {
			best = stem
		}
	}
	if best != "" {
		return best
	}

	// 3) Unresolvable — return as-is. Validate / apko publish will surface it.
	return streamVersion
}
