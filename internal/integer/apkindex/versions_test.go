package apkindex_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/verity-org/verity/internal/integer/apkindex"
)

var testPkgs = []apkindex.Package{
	{Name: "nodejs-16"},
	{Name: "nodejs-18"},
	{Name: "nodejs-20"},
	{Name: "nodejs-22"},
	{Name: "nodejs-24"},
	{Name: "nodejs-22-dev"}, // sub-package — should be excluded
	{Name: "nodejs-22-npm"}, // sub-package — should be excluded
	{Name: "go-1.23"},
	{Name: "go-1.24"},
	{Name: "go-1.25"},
	{Name: "go-1.26"},
	{Name: "postgresql-14"},
	{Name: "postgresql-15"},
	{Name: "postgresql-17"},
	{Name: "postgresql-17-client"}, // sub-package — should be excluded
	{Name: "curl"},
	{Name: "libcurl4"},
	{Name: "grafana-11"},
	{Name: "libstdc++"},
	{Name: "ca-certificates-bundle"},
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
