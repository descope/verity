package cmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"

	"github.com/verity-org/verity/internal/integer/apkindex"
	intconfig "github.com/verity-org/verity/internal/integer/config"
	"github.com/verity-org/verity/internal/integer/melange"
)

func TestPinLocalPackageVersionsPinsMatchingRenderedNames(t *testing.T) {
	tmpl := intconfig.TypeTemplate{Packages: []string{"cilium-{{version}}", "bash"}}

	err := pinLocalPackageVersions(&tmpl, "1.19", []apkindex.Package{
		{Name: "cilium-1.19", Version: "1.19.5-r5"},
		{Name: "cilium-1.19-cli", Version: "1.19.5-r5"},
	})

	require.NoError(t, err)
	assert.Equal(t, []string{"cilium-1.19=1.19.5-r5@local", "bash"}, tmpl.Packages)
}

func TestPinLocalPackageVersionsReplacesExistingConstraint(t *testing.T) {
	// Given: the image template pins an older revision of a locally built package.
	tmpl := intconfig.TypeTemplate{Packages: []string{"linkerd2-cli=25.12.3-r99", "bash"}}

	// When: the local repository contains the rebuilt package revision.
	err := pinLocalPackageVersions(&tmpl, "25", []apkindex.Package{
		{Name: "linkerd2-cli", Version: "25.12.3-r100"},
	})

	// Then: the local revision replaces the stale constraint.
	require.NoError(t, err)
	assert.Equal(t, []string{"linkerd2-cli=25.12.3-r100@local", "bash"}, tmpl.Packages)
}

func TestPinLocalPackageVersionsPinsLocalDependencyClosure(t *testing.T) {
	// Given: a locally built package depends on local subpackages that are absent from the image template.
	tmpl := intconfig.TypeTemplate{Packages: []string{"postgresql-15", "bash"}}
	packages := []apkindex.Package{
		{Name: "postgresql-15", Version: "15.14-r0", Origin: "postgresql-15", Dependencies: []string{"postgresql-15-base=15.14-r0", "tzdata"}},
		{Name: "postgresql-15-base", Version: "15.14-r0", Origin: "postgresql-15", Dependencies: []string{"so:libpq.so.5"}},
		{Name: "libpq-14", Version: "14.20-r2", Origin: "postgresql-14", Provides: []string{"so:libpq.so.5"}},
		{Name: "libpq-15", Version: "15.14-r0", Origin: "postgresql-15", Provides: []string{"so:libpq.so.5"}},
	}

	// When: local package versions are pinned.
	err := pinLocalPackageVersions(&tmpl, "15", packages)

	// Then: the root and every local dependency select the tagged repository exactly once.
	require.NoError(t, err)
	assert.Equal(t, []string{
		"postgresql-15=15.14-r0@local",
		"bash",
		"postgresql-15-base=15.14-r0@local",
		"libpq-15=15.14-r0@local",
	}, tmpl.Packages)
}

func TestPinLocalPackageVersionsPinsVirtualLocalDependency(t *testing.T) {
	// Given: a locally built subpackage provides the virtual dependency requested by the root package.
	tmpl := intconfig.TypeTemplate{Packages: []string{"application"}}
	packages := []apkindex.Package{
		{Name: "application", Version: "1.0-r0", Dependencies: []string{"so:libfoo.so.1"}},
		{Name: "libfoo", Version: "1.0-r0", Provides: []string{"so:libfoo.so.1"}},
	}

	// When: local package versions are pinned.
	err := pinLocalPackageVersions(&tmpl, "1", packages)

	// Then: the concrete provider is pinned from the local repository.
	require.NoError(t, err)
	assert.Equal(t, []string{"application=1.0-r0@local", "libfoo=1.0-r0@local"}, tmpl.Packages)
}

func TestPinLocalPackageVersionsRejectsConflictingVersions(t *testing.T) {
	tmpl := intconfig.TypeTemplate{Packages: []string{"cosign"}}

	err := pinLocalPackageVersions(&tmpl, "3", []apkindex.Package{
		{Name: "cosign", Version: "3.0.5-r1"},
		{Name: "cosign", Version: "3.1.1-r1"},
	})

	require.ErrorIs(t, err, errIntegerMelangePackageConflict)
}

func TestPinLocalPackageVersionsRejectsUnsatisfiedLocalDependency(t *testing.T) {
	// Given: a local dependency is older than the version required by its parent package.
	tmpl := intconfig.TypeTemplate{Packages: []string{"application"}}
	packages := []apkindex.Package{
		{Name: "application", Version: "1.0-r0", Dependencies: []string{"library>=2.0-r0"}},
		{Name: "library", Version: "1.0-r0"},
	}

	// When: local package versions are pinned.
	err := pinLocalPackageVersions(&tmpl, "1", packages)

	// Then: the build fails closed instead of silently selecting a remote dependency.
	require.ErrorIs(t, err, errIntegerMelangeDependencyConstraint)
}

func TestPinLocalPackageVersionsRejectsConfigWithoutLocalPackage(t *testing.T) {
	// Given: the bespoke repository does not provide any package requested by the image.
	tmpl := intconfig.TypeTemplate{Packages: []string{"haproxy-3.2", "bash"}}
	packages := []apkindex.Package{{Name: "haproxy", Version: "3.2.0-r0"}}

	// When: local package versions are pinned.
	err := pinLocalPackageVersions(&tmpl, "3.2", packages)

	// Then: the build fails closed instead of silently using the remote package.
	require.ErrorIs(t, err, errIntegerMelangePackageNotUsed)
}

func TestIntegerBuildCommandPinsLocalArtifactVersion(t *testing.T) {
	// Given: the local repository contains an older bespoke cosign build than
	// the remote repository may offer.
	repoRoot := t.TempDir()
	chdirIntegerMelangeTest(t, repoRoot)
	writeIntegerMelangeTestFile(repoRoot, "images/_base/wolfi-base.yaml", "# base\n")
	writeIntegerMelangeTestFile(repoRoot, "images/cosign.yaml", `
name: cosign
upstream:
  package: cosign
types:
  fips:
    base: wolfi-base
    packages: [cosign]
    melange:
      bespoke: cosign.yaml
versions:
  "3": {}
`)
	require.NoError(t, writeIntegerMelangeArtifactsWithPackages(t, &melange.BuildOptions{
		Paths: melange.DefaultPaths(repoRoot),
		Spec:  melange.Spec{Bespoke: []string{"cosign.yaml"}},
		Arch:  melange.ArchitectureX8664,
	}, []apkindex.Package{{Name: "cosign", Version: "3.0.5-r1"}}))
	capture := filepath.Join(repoRoot, "captured.apko.yaml")
	intCapturingApko(t, capture)

	// When: the normal image build prepares the existing bespoke artifacts.
	root := &cli.Command{Commands: []*cli.Command{IntegerCommand}}
	err := root.Run(context.Background(), []string{
		"verity", "integer", "build",
		"--image", "cosign",
		"--version", "3",
		"--type", "fips",
		"--images-dir", "images",
		"--apkindex-url", "",
		"--output", filepath.Join(repoRoot, "cosign.tar"),
	})

	// Then: apko receives an exact local version constraint, so a newer remote
	// candidate cannot outrank the bespoke artifact.
	require.NoError(t, err)
	rendered, err := os.ReadFile(capture)
	require.NoError(t, err)
	assert.Contains(t, string(rendered), "cosign=3.0.5-r1@local")
}
