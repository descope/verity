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
