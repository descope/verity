package apkindex

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPackageNameStripsRepositoryTagsAndVersionConstraints(t *testing.T) {
	tests := map[string]string{
		"crane":                         "crane",
		"crane=0.21.7-r1":               "crane",
		"crane=0.21.7-r1@local":         "crane",
		"tempo~2.10.0":                  "tempo",
		"linkerd2-cli>=25.12.3-r100":    "linkerd2-cli",
		"cilium-1.19<1.19.6-r0@local":   "cilium-1.19",
		"postgresql-18-client!=18.1-r0": "postgresql-18-client",
	}

	for packageSpec, expected := range tests {
		t.Run(packageSpec, func(t *testing.T) {
			assert.Equal(t, expected, PackageName(packageSpec))
		})
	}
}

func TestPackageSatisfiesConstraint(t *testing.T) {
	tests := map[string]struct {
		packageSpec string
		version     string
		expected    bool
	}{
		"unconstrained":     {packageSpec: "libpq-15", version: "15.14-r0", expected: true},
		"equal":             {packageSpec: "libpq-15=15.14-r0", version: "15.14-r0", expected: true},
		"greater":           {packageSpec: "libpq-15>15.13-r0", version: "15.14-r0", expected: true},
		"greater or equal":  {packageSpec: "libpq-15>=15.14-r0", version: "15.14-r0", expected: true},
		"less":              {packageSpec: "libpq-15<15.15-r0", version: "15.14-r0", expected: true},
		"less or equal":     {packageSpec: "libpq-15<=15.14-r0", version: "15.14-r0", expected: true},
		"fuzzy":             {packageSpec: "libpq-15~15.14", version: "15.14.2-r0", expected: true},
		"fuzzy digit guard": {packageSpec: "libpq-15~15.1", version: "15.14-r0", expected: false},
		"tagged":            {packageSpec: "libpq-15>=15.14-r0@local", version: "15.14-r0", expected: true},
		"unsatisfied":       {packageSpec: "libpq-15>=15.15-r0", version: "15.14-r0", expected: false},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			actual, err := PackageSatisfiesConstraint(test.packageSpec, test.version)
			require.NoError(t, err)
			assert.Equal(t, test.expected, actual)
		})
	}
}

func TestPackageSatisfiesConstraintRejectsInvalidConstraint(t *testing.T) {
	_, err := PackageSatisfiesConstraint("libpq-15!=15.14-r0", "15.14-r0")

	require.Error(t, err)
}

func TestResolveDependencyUsesUniqueLocalProvider(t *testing.T) {
	packages := map[string]Package{
		"application": {Name: "application", Version: "1.0-r0"},
		"libfoo":      {Name: "libfoo", Version: "1.0-r0", Provides: []string{"so:libfoo.so.1"}},
	}

	pkg, local, err := ResolveDependency(packages, "so:libfoo.so.1")

	require.NoError(t, err)
	assert.True(t, local)
	assert.Equal(t, "libfoo", pkg.Name)
}

func TestResolveDependencyRejectsAmbiguousLocalProvider(t *testing.T) {
	packages := map[string]Package{
		"libfoo":     {Name: "libfoo", Version: "1.0-r0", Provides: []string{"so:libfoo.so.1"}},
		"libfoo-alt": {Name: "libfoo-alt", Version: "1.0-r0", Provides: []string{"so:libfoo.so.1"}},
	}

	_, _, err := ResolveDependency(packages, "so:libfoo.so.1")

	require.Error(t, err)
}
