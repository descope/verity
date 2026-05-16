package render_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/verity-org/verity/internal/integer/config"
	"github.com/verity-org/verity/internal/integer/render"
)

var nodeDefault = config.TypeTemplate{
	Base:       "wolfi-base",
	Packages:   []string{"nodejs-{{version}}", "libstdc++"},
	Entrypoint: "/usr/bin/node",
	WorkDir:    "/app",
	Environment: map[string]string{
		"NODE_ENV": "production",
	},
	Paths: []config.PathDef{
		{Path: "/app", UID: 65532, GID: 65532, Permissions: "0o755"},
	},
}

func TestConfig_VersionSubstitution(t *testing.T) {
	out, err := render.Config(&nodeDefault, "22", "../../../_base")
	require.NoError(t, err)

	var cfg map[string]any
	require.NoError(t, yaml.Unmarshal(out, &cfg))

	// include directive
	assert.Equal(t, "../../../_base/wolfi-base.yaml", cfg["include"])

	// packages contain substituted version
	contents, ok := cfg["contents"].(map[string]any)
	require.True(t, ok, "expected contents key to be map[string]any")
	pkgs, ok := contents["packages"].([]any)
	require.True(t, ok, "expected packages key to be []any")
	assert.Equal(t, "nodejs-22", pkgs[0])
	assert.Equal(t, "libstdc++", pkgs[1])

	// entrypoint
	ep, ok := cfg["entrypoint"].(map[string]any)
	require.True(t, ok, "expected entrypoint key to be map[string]any")
	assert.Equal(t, "/usr/bin/node", ep["command"])

	// work-dir
	assert.Equal(t, "/app", cfg["work-dir"])

	// environment
	env, ok := cfg["environment"].(map[string]any)
	require.True(t, ok, "expected environment key to be map[string]any")
	assert.Equal(t, "production", env["NODE_ENV"])

	// paths
	paths, ok := cfg["paths"].([]any)
	require.True(t, ok, "expected paths key to be []any")
	require.Len(t, paths, 1)
	p, ok := paths[0].(map[string]any)
	require.True(t, ok, "expected path entry to be map[string]any")
	assert.Equal(t, "/app", p["path"])
	assert.Equal(t, "directory", p["type"])
	assert.Equal(t, 65532, p["uid"])
	assert.Equal(t, 65532, p["gid"])
}

func TestConfig_DifferentVersions(t *testing.T) {
	out22, err := render.Config(&nodeDefault, "22", "../../../_base")
	require.NoError(t, err)
	out24, err := render.Config(&nodeDefault, "24", "../../../_base")
	require.NoError(t, err)

	assert.True(t, strings.Contains(string(out22), "nodejs-22"))
	assert.True(t, strings.Contains(string(out24), "nodejs-24"))
	assert.False(t, strings.Contains(string(out22), "nodejs-24"))
}

func TestConfig_VersionInEnv(t *testing.T) {
	tmpl := config.TypeTemplate{
		Base:     "wolfi-base",
		Packages: []string{"postgresql-{{version}}", "postgresql-{{version}}-client"},
		Environment: map[string]string{
			"PG_MAJOR": "{{version}}",
			"PGDATA":   "/var/lib/postgresql/data",
		},
	}

	out, err := render.Config(&tmpl, "17", "../../../_base")
	require.NoError(t, err)

	var cfg map[string]any
	require.NoError(t, yaml.Unmarshal(out, &cfg))

	env, ok := cfg["environment"].(map[string]any)
	require.True(t, ok, "expected environment key to be map[string]any")
	assert.Equal(t, "17", env["PG_MAJOR"])
	assert.Equal(t, "/var/lib/postgresql/data", env["PGDATA"])

	contents, ok := cfg["contents"].(map[string]any)
	require.True(t, ok, "expected contents key to be map[string]any")
	pkgs, ok := contents["packages"].([]any)
	require.True(t, ok, "expected packages key to be []any")
	assert.Equal(t, "postgresql-17", pkgs[0])
	assert.Equal(t, "postgresql-17-client", pkgs[1])
}

func TestConfig_Unversioned(t *testing.T) {
	tmpl := config.TypeTemplate{
		Base:       "wolfi-base",
		Packages:   []string{"curl", "libcurl4"},
		Entrypoint: "/usr/bin/curl",
	}

	out, err := render.Config(&tmpl, "latest", "../../../_base")
	require.NoError(t, err)

	var cfg map[string]any
	require.NoError(t, yaml.Unmarshal(out, &cfg))

	contents, ok := cfg["contents"].(map[string]any)
	require.True(t, ok, "expected contents key to be map[string]any")
	pkgs, ok := contents["packages"].([]any)
	require.True(t, ok, "expected packages key to be []any")
	assert.Equal(t, "curl", pkgs[0])
	assert.Equal(t, "libcurl4", pkgs[1])
}

func TestConfig_NoPackages(t *testing.T) {
	tmpl := config.TypeTemplate{
		Base:       "wolfi-base",
		Entrypoint: "/bin/sh",
	}
	out, err := render.Config(&tmpl, "latest", "../../../_base")
	require.NoError(t, err)

	var cfg map[string]any
	require.NoError(t, yaml.Unmarshal(out, &cfg))
	assert.Nil(t, cfg["contents"])
}

func TestConfig_NoEntrypoint(t *testing.T) {
	tmpl := config.TypeTemplate{
		Base:     "wolfi-base",
		Packages: []string{"curl"},
	}
	out, err := render.Config(&tmpl, "latest", "../../../_base")
	require.NoError(t, err)

	var cfg map[string]any
	require.NoError(t, yaml.Unmarshal(out, &cfg))
	assert.Nil(t, cfg["entrypoint"])
}

func TestConfig_PathDefaultType(t *testing.T) {
	tmpl := config.TypeTemplate{
		Base: "wolfi-base",
		Paths: []config.PathDef{
			{Path: "/data", UID: 100, GID: 100},
		},
	}
	out, err := render.Config(&tmpl, "latest", "_base")
	require.NoError(t, err)

	var cfg map[string]any
	require.NoError(t, yaml.Unmarshal(out, &cfg))

	paths, ok := cfg["paths"].([]any)
	require.True(t, ok, "expected paths key to be []any")
	p, ok := paths[0].(map[string]any)
	require.True(t, ok, "expected path entry to be map[string]any")
	assert.Equal(t, "directory", p["type"])
	// directory entries must NOT emit a source field — apko rejects it.
	_, hasSource := p["source"]
	assert.False(t, hasSource, "directory path should not emit source field")
}

// TestConfig_PathSymlink covers the symlink path type added for #318 (A.2
// FHS-path mismatch). Charts that hardcode binaries at non-FHS paths (e.g.
// /usr/local/bin/etcd, /fluent-bit/bin/fluent-bit, /velero) are routed to
// verity's wolfi-FHS layout (/usr/bin/<x>) via these symlinks instead of
// duplicating binaries or modifying the upstream wolfi melange recipe.
//
// Regression guard: a previous schema rev had no Source field, so authors
// could not express symlinks at all and were forced into chart-command
// overrides per chart. This test fails against that schema and passes
// against the current one.
func TestConfig_PathSymlink(t *testing.T) {
	tmpl := config.TypeTemplate{
		Base: "wolfi-base",
		Paths: []config.PathDef{
			{Path: "/usr/local/bin/etcd", Type: "symlink", Source: "/usr/bin/etcd", UID: 0, GID: 0},
		},
	}
	out, err := render.Config(&tmpl, "latest", "_base")
	require.NoError(t, err)

	var cfg map[string]any
	require.NoError(t, yaml.Unmarshal(out, &cfg))

	paths, ok := cfg["paths"].([]any)
	require.True(t, ok, "expected paths key to be []any")
	require.Len(t, paths, 1)
	p, ok := paths[0].(map[string]any)
	require.True(t, ok, "expected path entry to be map[string]any")
	assert.Equal(t, "/usr/local/bin/etcd", p["path"])
	assert.Equal(t, "symlink", p["type"])
	assert.Equal(t, "/usr/bin/etcd", p["source"])
	assert.Equal(t, 0, p["uid"])
	assert.Equal(t, 0, p["gid"])
}

// TestConfig_PathSymlink_VersionSubstitution confirms {{version}} substitution
// applies inside Source (so version-templated symlink targets work).
func TestConfig_PathSymlink_VersionSubstitution(t *testing.T) {
	tmpl := config.TypeTemplate{
		Base: "wolfi-base",
		Paths: []config.PathDef{
			{Path: "/usr/local/bin/etcd-{{version}}", Type: "symlink", Source: "/usr/bin/etcd-{{version}}", UID: 0, GID: 0},
		},
	}
	out, err := render.Config(&tmpl, "3.6", "_base")
	require.NoError(t, err)

	var cfg map[string]any
	require.NoError(t, yaml.Unmarshal(out, &cfg))

	paths, ok := cfg["paths"].([]any)
	require.True(t, ok, "expected paths key to be []any")
	require.Len(t, paths, 1)
	p, ok := paths[0].(map[string]any)
	require.True(t, ok, "expected path entry to be map[string]any")
	assert.Equal(t, "/usr/local/bin/etcd-3.6", p["path"])
	assert.Equal(t, "/usr/bin/etcd-3.6", p["source"])
}

// TestConfig_PathSymlink_RequiresSource is a regression guard: type=symlink
// without a Source field is misconfigured and apko would reject it at build
// time. Render should fail fast with a clear error instead. (Caught by code
// review on PR #330 — the renderer originally accepted this silently.)
func TestConfig_PathSymlink_RequiresSource(t *testing.T) {
	tmpl := config.TypeTemplate{
		Base: "wolfi-base",
		Paths: []config.PathDef{
			{Path: "/usr/local/bin/etcd", Type: "symlink", UID: 0, GID: 0}, // no Source
		},
	}
	_, err := render.Config(&tmpl, "latest", "_base")
	require.Error(t, err)
	assert.ErrorIs(t, err, render.ErrSymlinkRequiresSource)
	assert.Contains(t, err.Error(), "/usr/local/bin/etcd")
}

// TestConfig_PathSourceOnNonSymlink_Errors is the inverse regression guard:
// Source is only valid for type=symlink. Any other type with Source set is a
// misconfiguration; apko rejects it at build. Render should fail fast.
// (Caught by code review on PR #330 — same root cause as RequiresSource.)
func TestConfig_PathSourceOnNonSymlink_Errors(t *testing.T) {
	tmpl := config.TypeTemplate{
		Base: "wolfi-base",
		Paths: []config.PathDef{
			// type defaults to "directory" — Source on a directory is invalid.
			{Path: "/usr/local/bin/etcd", Source: "/usr/bin/etcd", UID: 0, GID: 0},
		},
	}
	_, err := render.Config(&tmpl, "latest", "_base")
	require.Error(t, err)
	assert.ErrorIs(t, err, render.ErrSourceOnNonSymlink)
	assert.Contains(t, err.Error(), "/usr/local/bin/etcd")
	assert.Contains(t, err.Error(), `type="directory"`) // ensure the actual type is reported
}

// TestConfig_SymlinkBeforePermissions_ApkoChmodQuirk regression-tests the
// reorder in `convertPaths` for the apko `mutatePermissions on every
// non-permissions entry` quirk (see the convertPaths docstring). When a
// permissions mutation on `/usr/bin/X` is declared BEFORE a symlink
// mutation pointing at `/usr/bin/X`, the rendered apko config MUST emit
// the symlink first so apko's implicit Chmod(0) on the symlink doesn't
// follow the link and reset the target's mode to 0o000.
//
// This protects the etcd / velero / fluent-bit FHS-symlink images from
// silently regressing if a maintainer reorders entries in images/*.yaml
// to read more naturally.
func TestConfig_SymlinkBeforePermissions_ApkoChmodQuirk(t *testing.T) {
	tmpl := config.TypeTemplate{
		Base: "wolfi-base",
		// Source ORDER intentionally puts permissions before symlink —
		// the test asserts the renderer rewrites it.
		Paths: []config.PathDef{
			{Path: "/var/lib/etcd", Type: "directory", UID: 65532, GID: 65532, Permissions: "0o700"},
			{Path: "/usr/bin/etcd", Type: "permissions", UID: 0, GID: 0, Permissions: "0o755"},
			{Path: "/usr/local/bin/etcd", Type: "symlink", Source: "/usr/bin/etcd", UID: 0, GID: 0},
		},
	}
	out, err := render.Config(&tmpl, "latest", "_base")
	require.NoError(t, err)

	var cfg map[string]any
	require.NoError(t, yaml.Unmarshal(out, &cfg))

	paths, ok := cfg["paths"].([]any)
	require.True(t, ok)
	require.Len(t, paths, 3)

	// The symlink whose source matches the permissions entry must come
	// FIRST in the emitted order (rewritten from its source position).
	first, ok := paths[0].(map[string]any)
	require.True(t, ok, "paths[0] is map[string]any")
	assert.Equal(t, "symlink", first["type"], "symlink that targets a permissions-mutated path must be emitted first")
	assert.Equal(t, "/usr/local/bin/etcd", first["path"])
	assert.Equal(t, "/usr/bin/etcd", first["source"])

	// Other entries retain their relative ordering.
	second, ok := paths[1].(map[string]any)
	require.True(t, ok, "paths[1] is map[string]any")
	assert.Equal(t, "directory", second["type"])
	assert.Equal(t, "/var/lib/etcd", second["path"])

	third, ok := paths[2].(map[string]any)
	require.True(t, ok, "paths[2] is map[string]any")
	assert.Equal(t, "permissions", third["type"])
	assert.Equal(t, "/usr/bin/etcd", third["path"])
}

// TestConfig_UnrelatedSymlinkOrderPreserved confirms the reorder only
// touches symlinks whose source matches a permissions entry. A symlink
// pointing at an unrelated path keeps its original position.
func TestConfig_UnrelatedSymlinkOrderPreserved(t *testing.T) {
	tmpl := config.TypeTemplate{
		Base: "wolfi-base",
		Paths: []config.PathDef{
			{Path: "/var/lib/foo", Type: "directory", UID: 65532, GID: 65532, Permissions: "0o755"},
			{Path: "/usr/bin/foo", Type: "permissions", UID: 0, GID: 0, Permissions: "0o755"},
			// Symlink pointing at /usr/bin/bar — NOT matching any permissions entry.
			{Path: "/usr/local/bin/bar", Type: "symlink", Source: "/usr/bin/bar", UID: 0, GID: 0},
		},
	}
	out, err := render.Config(&tmpl, "latest", "_base")
	require.NoError(t, err)

	var cfg map[string]any
	require.NoError(t, yaml.Unmarshal(out, &cfg))

	paths, ok := cfg["paths"].([]any)
	require.True(t, ok)
	require.Len(t, paths, 3)

	// Order is unchanged because no symlink target matches a permissions path.
	first, ok := paths[0].(map[string]any)
	require.True(t, ok, "paths[0] is map[string]any")
	assert.Equal(t, "/var/lib/foo", first["path"])
	second, ok := paths[1].(map[string]any)
	require.True(t, ok, "paths[1] is map[string]any")
	assert.Equal(t, "/usr/bin/foo", second["path"])
	third, ok := paths[2].(map[string]any)
	require.True(t, ok, "paths[2] is map[string]any")
	assert.Equal(t, "/usr/local/bin/bar", third["path"])
}
