package melange

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	intconfig "github.com/verity-org/verity/internal/integer/config"
)

func TestResolveSpecSubstitutesVersion(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "images", "cilium.yaml"), `
name: cilium
description: cilium
upstream:
  package: cilium-{{version}}
types:
  default:
    base: wolfi-base
    packages: ["cilium-{{version}}"]
    melange:
      upstream: cilium-{{version}}
      env-file: fips-{{version}}.env
      build-option: stream-{{version}}
versions:
  "1.19": {}
`)

	spec, err := ResolveSpec(filepath.Join(root, "images"), "cilium", "1.19", "default")
	require.NoError(t, err)
	assert.Equal(t, Spec{
		Upstream:    "cilium-1.19",
		EnvFile:     "fips-1.19.env",
		BuildOption: "stream-1.19",
	}, spec)
}

func TestResolveSpecReturnsEmptyForStandardImage(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "images", "curl.yaml"), `
name: curl
description: curl
upstream:
  package: curl
types:
  default:
    base: wolfi-base
    packages: ["curl"]
versions:
  latest: {}
`)

	spec, err := ResolveSpec(filepath.Join(root, "images"), "curl", "latest", "default")
	require.NoError(t, err)
	assert.False(t, spec.Needed())
}

func TestResolveSpecScopesMelangeByVersion(t *testing.T) {
	// Given: only PostgreSQL 14 and 15 declare bespoke recipes for the default type.
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "images", "postgres.yaml"), `
name: postgres
description: postgres
upstream:
  package: postgresql-{{version}}
types:
  default:
    base: wolfi-base
    packages: ["postgresql-{{version}}"]
versions:
  "14":
    melange:
      default:
        bespoke: postgresql-14.yaml
  "15":
    melange:
      default:
        bespoke: postgresql-15.yaml
  "16": {}
`)

	tests := []struct {
		version string
		want    Spec
	}{
		{version: "14", want: Spec{Bespoke: []string{"postgresql-14.yaml"}}},
		{version: "15", want: Spec{Bespoke: []string{"postgresql-15.yaml"}}},
		{version: "16", want: Spec{}},
	}
	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			// When: the default type's Melange input is resolved for one stream.
			got, err := ResolveSpec(filepath.Join(root, "images"), "postgres", tt.version, "default")

			// Then: only that stream's scoped recipe is selected.
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestResolveSpecReturnsEmptyForAutoDiscoveredStandardVersion(t *testing.T) {
	// Given: a standard image whose APKINDEX-discovered version is newer than
	// the explicitly curated versions map.
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "images", "traefik.yaml"), `
name: traefik
description: traefik
upstream:
  package: traefik-{{version}}
types:
  default:
    base: wolfi-base
    packages: ["traefik-{{version}}"]
versions:
  "3.6": {}
`)

	// When: the unconditional workflow preparation step resolves the
	// auto-discovered version.
	spec, err := ResolveSpec(filepath.Join(root, "images"), "traefik", "3.7", "default")

	// Then: no bespoke build is required and preparation remains a no-op.
	require.NoError(t, err)
	assert.False(t, spec.Needed())
}

func TestResolveSpecFindsNestedDefinitionByDeclaredName(t *testing.T) {
	// Given: a nested definition whose file path and declared name differ.
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "images", "platform", "custom-file.yaml"), `
name: renamed-image
description: renamed image
upstream:
  package: renamed-image
types:
  default:
    base: wolfi-base
    packages: ["renamed-image"]
    melange:
      bespoke: renamed-image.yaml
versions:
  latest: {}
`)

	// When: the build spec is resolved by declared image name.
	spec, err := ResolveSpec(filepath.Join(root, "images"), "renamed-image", "latest", "default")

	// Then: the nested definition supplies the bespoke recipe.
	require.NoError(t, err)
	assert.Equal(t, Spec{Bespoke: []string{"renamed-image.yaml"}}, spec)
}

func TestWriteGitHubOutput(t *testing.T) {
	var out bytes.Buffer
	err := WriteGitHubOutput(&out, Spec{Upstream: "caddy", EnvFile: "fips.env", BuildOption: "fips"})
	require.NoError(t, err)
	assert.Equal(t, "needed=true\nenv_file=fips.env\nbuild_option=fips\n", out.String())
}

func TestResolveConfigSpecRejectsTraversalBasenames(t *testing.T) {
	for _, file := range []string{".", ".."} {
		t.Run(file, func(t *testing.T) {
			_, err := ResolveConfigSpec(&intconfig.MelangeSpec{Bespoke: []string{file}}, "1.0.0")
			require.ErrorIs(t, err, errInvalidBespokeFilename)
		})
	}
}

func TestResolveConfigSpecSupportsVersionBuildMetadata(t *testing.T) {
	spec, err := ResolveConfigSpec(&intconfig.MelangeSpec{
		Upstream:    "package-{{version}}",
		EnvFile:     "env-{{version}}.env",
		BuildOption: "build-{{version}}",
	}, "1.2.3+fips")
	require.NoError(t, err)
	assert.Equal(t, Spec{
		Upstream:    "package-1.2.3+fips",
		EnvFile:     "env-1.2.3+fips.env",
		BuildOption: "build-1.2.3+fips",
	}, spec)
}
