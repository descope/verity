package apkindex_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/verity-org/verity/internal/integer/apkindex"
)

var testPkgs = []apkindex.Package{
	{Name: "nodejs-16", Version: "16.20.2-r0"},
	{Name: "nodejs-18", Version: "18.20.8-r0"},
	{Name: "nodejs-20", Version: "20.19.2-r0"},
	{Name: "nodejs-22", Version: "22.16.0-r0"},
	{Name: "nodejs-24", Version: "24.1.0-r1"},
	{Name: "nodejs-22-dev", Version: "22.16.0-r0"}, // sub-package — should be excluded
	{Name: "nodejs-22-npm", Version: "22.16.0-r0"}, // sub-package — should be excluded
	{Name: "go-1.23", Version: "1.23.8-r0"},
	{Name: "go-1.24", Version: "1.24.3-r0"},
	{Name: "go-1.25", Version: "1.25.1-r0"},
	{Name: "go-1.26", Version: "1.26.0-r0"},
	{Name: "postgresql-14", Version: "14.18.0-r0"},
	{Name: "postgresql-15", Version: "15.13.0-r0"},
	{Name: "postgresql-17", Version: "17.5.0-r0"},
	{Name: "postgresql-17-client", Version: "17.5.0-r0"}, // sub-package — should be excluded
	{Name: "curl", Version: "8.13.0-r0"},
	{Name: "libcurl4", Version: "8.13.0-r0"},
	{Name: "grafana-11", Version: "11.6.0-r0"},
	{Name: "libstdc++", Version: "14.2.1-r0"},
	{Name: "ca-certificates-bundle", Version: "20250320-r0"},
}

func TestDiscoverVersions_Versioned(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		expected []string
	}{
		{
			name:     "nodejs",
			pattern:  "nodejs-{{version}}",
			expected: []string{"16", "18", "20", "22", "24"},
		},
		{
			name:     "go (dotted versions)",
			pattern:  "go-{{version}}",
			expected: []string{"1.23", "1.24", "1.25", "1.26"},
		},
		{
			name:     "postgresql",
			pattern:  "postgresql-{{version}}",
			expected: []string{"14", "15", "17"},
		},
		{
			name:     "grafana",
			pattern:  "grafana-{{version}}",
			expected: []string{"11"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := apkindex.DiscoverVersions(testPkgs, tt.pattern)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestDiscoverVersions_Unversioned(t *testing.T) {
	t.Run("package exists", func(t *testing.T) {
		got := apkindex.DiscoverVersions(testPkgs, "curl")
		assert.Equal(t, []string{"latest"}, got)
	})

	t.Run("package does not exist", func(t *testing.T) {
		got := apkindex.DiscoverVersions(testPkgs, "nonexistent")
		assert.Empty(t, got)
	})
}

func TestDiscoverVersions_NumericalSort(t *testing.T) {
	pkgs := []apkindex.Package{
		{Name: "nodejs-8"},
		{Name: "nodejs-10"},
		{Name: "nodejs-9"},
		{Name: "nodejs-20"},
	}
	got := apkindex.DiscoverVersions(pkgs, "nodejs-{{version}}")
	assert.Equal(t, []string{"8", "9", "10", "20"}, got)
}

func TestDiscoverVersions_EmptyPackages(t *testing.T) {
	got := apkindex.DiscoverVersions(nil, "nodejs-{{version}}")
	assert.Empty(t, got)
}

func TestVersionLess(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"1.9", "1.10", true},
		{"1.10", "1.9", false},
		{"20", "22", true},
		{"22", "20", false},
		{"1.0", "1.0", false},
		{"3.12", "3.13", true},
		{"3.13", "3.12", false},
	}
	for _, tt := range tests {
		t.Run(tt.a+"_vs_"+tt.b, func(t *testing.T) {
			assert.Equal(t, tt.want, apkindex.VersionLess(tt.a, tt.b))
		})
	}
}

func TestLookupFullVersion(t *testing.T) {
	tests := []struct {
		name        string
		packageName string
		expected    string
	}{
		{"found unversioned", "curl", "8.13.0"},
		{"found versioned", "nodejs-22", "22.16.0"},
		{"strips -r1 suffix", "nodejs-24", "24.1.0"},
		{"not found", "nonexistent", ""},
		{"date-like version", "ca-certificates-bundle", "20250320"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := apkindex.LookupFullVersion(testPkgs, tt.packageName)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestLookupFullVersion_MultipleEntries(t *testing.T) {
	pkgs := []apkindex.Package{
		{Name: "caddy", Version: "2.11.1-r0"},
		{Name: "caddy", Version: "2.11.2-r0"},
	}
	got := apkindex.LookupFullVersion(pkgs, "caddy")
	assert.Equal(t, "2.11.2", got)
}

func TestLookupFullVersion_NoRevisionSuffix(t *testing.T) {
	pkgs := []apkindex.Package{
		{Name: "test", Version: "1.2.3"},
	}
	got := apkindex.LookupFullVersion(pkgs, "test")
	assert.Equal(t, "1.2.3", got)
}

func TestResolveFullVersion(t *testing.T) {
	tests := []struct {
		name          string
		pattern       string
		streamVersion string
		expected      string
	}{
		{"versioned pattern", "nodejs-{{version}}", "22", "22.16.0"},
		{"versioned dotted", "go-{{version}}", "1.24", "1.24.3"},
		{"versioned not found", "nodejs-{{version}}", "99", ""},
		{"unversioned", "curl", "8", "8.13.0"},
		{"unversioned latest", "curl", "latest", "8.13.0"},
		{"unversioned not found", "nonexistent", "1", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := apkindex.ResolveFullVersion(testPkgs, tt.pattern, tt.streamVersion)
			assert.Equal(t, tt.expected, got)
		})
	}
}

// TestResolveAliasVersion guards the floating-major → APK alias path that
// fixes the chronic Integer Build Image failures
// (`failed to build image components: ... nothing provides "kyverno-1"`).
//
// Wolfi APKINDEX naming is uneven: some packages publish a per-major meta
// (e.g. nodejs-22, go-1.24) but others only publish a specific minor
// (kyverno-1.17, no kyverno-1; cilium-1.18, no cilium-1; …). When the image
// config declares a floating-major stream like `versions: { "1": {} }`, the
// renderer must alias "1" → highest-matching-minor so apko's apk solver can
// satisfy the constraint at publish time.
func TestResolveAliasVersion(t *testing.T) {
	pkgs := []apkindex.Package{
		// Floating-major case A: only minors exist, no meta-package.
		// This is the exact shape of the kyverno bug reported in the
		// failing run 25254581240.
		{Name: "kyverno-1.17", Version: "1.17.5-r0"},
		{Name: "kyverno-1.16", Version: "1.16.3-r0"},

		// Floating-major case B: multiple minors under the same major,
		// alias must pick the highest. Mirrors prometheus-2.55, fluent-bit-3.x.
		{Name: "prometheus-2.54", Version: "2.54.1-r0"},
		{Name: "prometheus-2.55", Version: "2.55.0-r0"},

		// Working precedent: literal exists.
		{Name: "nodejs-22", Version: "22.16.0-r0"},
		{Name: "nodejs-22-dev", Version: "22.16.0-r0"}, // sub-pkg, must not alias

		// Working precedent: dotted stream exists literally.
		{Name: "go-1.24", Version: "1.24.3-r0"},

		// Sub-packages that look like minors but aren't valid stems:
		// renderer must NOT alias to "envoy-gateway".
		{Name: "envoy-1.34", Version: "1.34.1-r0"},
		{Name: "envoy-gateway", Version: "1.4.0-r0"},

		// Unversioned package — alias must be a no-op.
		{Name: "popeye", Version: "0.22.1-r0"},
	}

	tests := []struct {
		name     string
		pattern  string
		stream   string
		expected string
	}{
		{
			name:     "floating-major aliases to highest minor (kyverno bug)",
			pattern:  "kyverno-{{version}}",
			stream:   "1",
			expected: "1.17",
		},
		{
			name:     "alias picks highest minor among multiple",
			pattern:  "prometheus-{{version}}",
			stream:   "2",
			expected: "2.55",
		},
		{
			name:     "literal stream matches without aliasing",
			pattern:  "nodejs-{{version}}",
			stream:   "22",
			expected: "22",
		},
		{
			name:     "literal dotted stream matches without aliasing",
			pattern:  "go-{{version}}",
			stream:   "1.24",
			expected: "1.24",
		},
		{
			name:     "specific minor matches literally even when major missing",
			pattern:  "kyverno-{{version}}",
			stream:   "1.17",
			expected: "1.17",
		},
		{
			name:     "unresolvable stream returned as-is",
			pattern:  "kyverno-{{version}}",
			stream:   "99",
			expected: "99",
		},
		{
			name:     "alias ignores sibling package suffixes",
			pattern:  "envoy-{{version}}",
			stream:   "1",
			expected: "1.34", // must NOT alias to "gateway"
		},
		{
			name:     "unversioned upstream pattern is a no-op",
			pattern:  "popeye",
			stream:   "0.22.1",
			expected: "0.22.1",
		},
		{
			name:     "empty stream returns empty",
			pattern:  "kyverno-{{version}}",
			stream:   "",
			expected: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := apkindex.ResolveAliasVersion(pkgs, tt.pattern, tt.stream)
			assert.Equal(t, tt.expected, got)
		})
	}
}

// TestResolveAliasVersion_NilPackages verifies that aliasing is a no-op
// when the APKINDEX is unavailable (offline `verity integer discover`).
// The pre-fix behaviour — substitute the declared stream verbatim — must
// be preserved so generated apko configs stay deterministic offline.
func TestResolveAliasVersion_NilPackages(t *testing.T) {
	got := apkindex.ResolveAliasVersion(nil, "kyverno-{{version}}", "1")
	assert.Equal(t, "1", got)
}
