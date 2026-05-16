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

// TestConfig_SymlinkInheritsTargetPerms_ApkoChmodQuirk regression-tests
// the permissions-propagation in `convertPaths` for the apko
// `mutatePermissions on every non-permissions entry` quirk (see the
// convertPaths docstring). When a permissions mutation on `/usr/bin/X`
// declares mode 0o755 and a symlink points at `/usr/bin/X`, the rendered
// apko config MUST copy that 0o755 onto the symlink entry so apko's
// implicit Chmod on the symlink (which follows the link) writes 0o755
// to the target instead of 0.
//
// This approach has zero ordering impact — parent-directory-before-
// symlink invariants and same-target-permissions declaration order are
// both preserved.
func TestConfig_SymlinkInheritsTargetPerms_ApkoChmodQuirk(t *testing.T) {
	tmpl := config.TypeTemplate{
		Base: "wolfi-base",
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

	// Order is COMPLETELY UNCHANGED — every entry stays at its
	// original index. The fix is permissions-propagation, not
	// reordering.
	p0, ok := paths[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "directory", p0["type"])
	assert.Equal(t, "/var/lib/etcd", p0["path"])

	p1, ok := paths[1].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "permissions", p1["type"])
	assert.Equal(t, "/usr/bin/etcd", p1["path"])

	p2, ok := paths[2].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "symlink", p2["type"])
	assert.Equal(t, "/usr/local/bin/etcd", p2["path"])
	assert.Equal(t, "/usr/bin/etcd", p2["source"])
	// The symlink inherited the target's permissions 0o755 = 493
	// decimal. apko's implicit Chmod on the symlink now writes 0o755
	// to the target instead of 0.
	assert.Equal(t, 493, p2["permissions"], "symlink inherits its target's permissions mutation value")
}

// TestConfig_SymlinkParentDirAndPermsOrderingPreserved guards the
// fluent-bit invariant: a parent-directory entry created before a
// symlink hosted in that directory keeps its position-before-symlink
// ordering. Since the fix is propagation (not reorder), all 4 entries
// stay at their declared indices.
func TestConfig_SymlinkParentDirAndPermsOrderingPreserved(t *testing.T) {
	tmpl := config.TypeTemplate{
		Base: "wolfi-base",
		// Shape mirrors images/fluent-bit.yaml.
		Paths: []config.PathDef{
			{Path: "/etc/fluent-bit", Type: "directory", UID: 65532, GID: 65532, Permissions: "0o755"},
			{Path: "/fluent-bit/bin", Type: "directory", UID: 0, GID: 0, Permissions: "0o755"},
			{Path: "/usr/bin/fluent-bit", Type: "permissions", UID: 0, GID: 0, Permissions: "0o755"},
			{Path: "/fluent-bit/bin/fluent-bit", Type: "symlink", Source: "/usr/bin/fluent-bit", UID: 0, GID: 0},
		},
	}
	out, err := render.Config(&tmpl, "latest", "_base")
	require.NoError(t, err)

	var cfg map[string]any
	require.NoError(t, yaml.Unmarshal(out, &cfg))

	paths, ok := cfg["paths"].([]any)
	require.True(t, ok)
	require.Len(t, paths, 4)

	// Indices unchanged.
	for i, want := range []struct {
		ptype string
		path  string
	}{
		{"directory", "/etc/fluent-bit"},
		{"directory", "/fluent-bit/bin"},
		{"permissions", "/usr/bin/fluent-bit"},
		{"symlink", "/fluent-bit/bin/fluent-bit"},
	} {
		p, ok := paths[i].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, want.ptype, p["type"], "paths[%d].type", i)
		assert.Equal(t, want.path, p["path"], "paths[%d].path", i)
	}

	// Symlink inherited the target's perms.
	sym, ok := paths[3].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, 493, sym["permissions"])
}

// TestConfig_ExplicitSymlinkPermsRespected confirms the propagation only
// fires when the symlink's own permissions are unset (mode 0). If a YAML
// author explicitly set the symlink's permissions, that wins.
func TestConfig_ExplicitSymlinkPermsRespected(t *testing.T) {
	tmpl := config.TypeTemplate{
		Base: "wolfi-base",
		Paths: []config.PathDef{
			{Path: "/usr/bin/foo", Type: "permissions", UID: 0, GID: 0, Permissions: "0o755"},
			// Author explicitly set 0o700 on the symlink.
			{Path: "/usr/local/bin/foo", Type: "symlink", Source: "/usr/bin/foo", UID: 0, GID: 0, Permissions: "0o700"},
		},
	}
	out, err := render.Config(&tmpl, "latest", "_base")
	require.NoError(t, err)

	var cfg map[string]any
	require.NoError(t, yaml.Unmarshal(out, &cfg))

	paths, ok := cfg["paths"].([]any)
	require.True(t, ok)
	require.Len(t, paths, 2)

	sym, ok := paths[1].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "symlink", sym["type"])
	// 0o700 = 448 decimal. Author's explicit value wins over the
	// propagation (which would have written 0o755 = 493).
	assert.Equal(t, 448, sym["permissions"], "explicit symlink permissions are not overwritten by propagation")
}

// TestConfig_UnrelatedSymlinkPermsZero confirms a symlink that doesn't
// target any permissions-mutated path stays at permissions: 0. This is
// the prior behaviour for the common case (most symlinks).
func TestConfig_UnrelatedSymlinkPermsZero(t *testing.T) {
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

	// Order unchanged.
	p0, ok := paths[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "/var/lib/foo", p0["path"])
	p2, ok := paths[2].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "/usr/local/bin/bar", p2["path"])
	// Unrelated symlink keeps its zero permissions (no propagation
	// source).
	assert.NotContains(t, p2, "permissions", "unrelated symlink omits permissions key (zero value, omitempty)")
}
