package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/integer/apkindex"
	intconfig "github.com/verity-org/verity/internal/integer/config"
)

func Test_PgBouncer_uses_pinned_bespoke_package(t *testing.T) {
	// Given: the checked-in PgBouncer image definition.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "pgbouncer.yaml"))
	require.NoError(t, err)

	// When: the approved default image variant is resolved.
	tmpl := def.Types["default"]

	// Then: the image selects the local rebuild and preserves its nonroot runtime contract.
	require.NotNil(t, tmpl.Melange)
	require.Equal(t, "pgbouncer.yaml", tmpl.Melange.Bespoke.First())
	require.Equal(t, []string{"pgbouncer"}, tmpl.Packages)
	require.Equal(t, "/usr/bin/pgbouncer /etc/pgbouncer/pgbouncer.ini", tmpl.Entrypoint)
	require.Contains(t, tmpl.Paths, intconfig.PathDef{
		Path:        "/tmp/pgbouncer",
		Type:        "directory",
		UID:         65532,
		GID:         65532,
		Permissions: "0o755",
	})
	require.Contains(t, def.Versions, "1")
}

func Test_PgBouncer_recipe_pins_immutable_release(t *testing.T) {
	// Given: the bespoke package recipe selected by the image.
	recipe, err := os.ReadFile(filepath.Join("..", "packages", "bespoke", "pgbouncer.yaml"))
	require.NoError(t, err)
	text := string(recipe)

	// When: its source, build, runtime, test, configuration, and license metadata are inspected.

	// Then: revision r0 rebuilds the immutable ISC-licensed 1.25.2 security release.
	require.Contains(t, text, "name: pgbouncer")
	require.Contains(t, text, "version: \"1.25.2\"")
	require.Contains(t, text, "epoch: 0")
	require.Contains(t, text, "license: ISC")
	require.Contains(t, text, "515b6284d273bfd62d1b7e3931ccdce41f9bfe7b")
	require.Contains(t, text, "pgbouncer-${{package.version}}.tar.gz")
	require.Contains(t, text, "expected-sha256: 924ad35113fd0a71c8e2dbe85b5d03445532e2b7b37a9f8a48983beea238b332")
	require.Contains(t, text, "make -j$(nproc) pgbouncer")
	require.NotContains(t, text, "pandoc")
	require.Contains(t, text, "usr/share/licenses/${{package.name}}/COPYRIGHT")
	require.Contains(t, text, "etc/pgbouncer/pgbouncer.ini")
	require.Contains(t, text, "etc/pgbouncer/userlist.txt")
	require.Contains(t, text, "listen_addr = 0.0.0.0")
	require.Contains(t, text, "pgbouncer --version")
	require.Contains(t, text, "test -S /tmp/pgbouncer/.s.PGSQL.6432")
	require.Contains(t, text, "apk info --license pgbouncer")
}

func Test_PgBouncer_resolves_fixed_local_package(t *testing.T) {
	// Given: the checked-in image template and the package produced by the bespoke recipe.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "pgbouncer.yaml"))
	require.NoError(t, err)
	tmpl := def.Types["default"]

	// When: the local Melange artifact is pinned for the image build.
	err = pinLocalPackageVersions(&tmpl, "1", []apkindex.Package{{
		Name:    "pgbouncer",
		Version: "1.25.2-r0",
	}})

	// Then: apko can only select the approved locally built security release.
	require.NoError(t, err)
	require.Equal(t, []string{"pgbouncer=1.25.2-r0@local"}, tmpl.Packages)
}
