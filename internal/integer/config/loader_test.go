package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/integer/config"
)

const globalConfig = `
target:
  registry: ghcr.io/verity-org
defaults:
  archs: [amd64, arm64]
`

const nodeImageYAML = `
name: node
description: "Node.js runtime"
upstream:
  package: "nodejs-{{version}}"
types:
  default:
    base: wolfi-base
    packages: ["nodejs-{{version}}", "libstdc++"]
    entrypoint: /usr/bin/node
    cmd: --version
    work-dir: /app
    environment:
      NODE_ENV: production
    paths:
      - path: /app
        uid: 65532
        gid: 65532
        permissions: "0o755"
  fips:
    base: wolfi-fips
    fips-profile: openssl
    packages: ["nodejs-{{version}}", "libstdc++"]
    entrypoint: /usr/bin/node
versions:
  "22":
    eol: "2027-04-30"
  "24":
    eol: "2028-04-30"
    latest: true
`

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "integer.yaml", globalConfig)

	cfg, err := config.LoadConfig(path)
	require.NoError(t, err)

	assert.Equal(t, "ghcr.io/verity-org", cfg.Target.Registry)
	assert.Equal(t, []string{"amd64", "arm64"}, cfg.Defaults.Archs)
}

func TestLoadConfig_NotFound(t *testing.T) {
	_, err := config.LoadConfig("/nonexistent/integer.yaml")
	require.Error(t, err)
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "integer.yaml", "not: valid: yaml: [")
	_, err := config.LoadConfig(path)
	require.Error(t, err)
}

func TestLoadImage(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "node.yaml", nodeImageYAML)

	def, err := config.LoadImage(path)
	require.NoError(t, err)

	assert.Equal(t, "node", def.Name)
	assert.Equal(t, "Node.js runtime", def.Description)
	assert.Equal(t, "nodejs-{{version}}", def.Upstream.Package)

	require.Len(t, def.Types, 2)

	dflt := def.Types["default"]
	assert.Equal(t, "wolfi-base", dflt.Base)
	assert.Equal(t, []string{"nodejs-{{version}}", "libstdc++"}, dflt.Packages)
	assert.Equal(t, "/usr/bin/node", dflt.Entrypoint)
	assert.Equal(t, "--version", dflt.Cmd)
	assert.Equal(t, "/app", dflt.WorkDir)
	assert.Equal(t, "production", dflt.Environment["NODE_ENV"])
	require.Len(t, dflt.Paths, 1)
	assert.Equal(t, "/app", dflt.Paths[0].Path)
	assert.Equal(t, 65532, dflt.Paths[0].UID)

	fips := def.Types["fips"]
	assert.Equal(t, "wolfi-fips", fips.Base)
	assert.Equal(t, config.FIPSProfileOpenSSL, fips.FIPSProfile)

	require.Len(t, def.Versions, 2)
	v22 := def.Versions["22"]
	assert.Equal(t, "2027-04-30", v22.EOL)
	assert.False(t, v22.Latest)

	v24 := def.Versions["24"]
	assert.Equal(t, "2028-04-30", v24.EOL)
	assert.True(t, v24.Latest)
}

func TestLoadImage_AllowsAliasedBespokeListEntries(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "aliased.yaml", `
name: custom
description: "Custom image"
upstream:
  package: custom
types:
  default:
    base: wolfi-base
    melange:
      bespoke:
        - &spec custom.melange.yaml
        - *spec
`)

	def, err := config.LoadImage(path)
	require.NoError(t, err)
	require.NotNil(t, def.Types["default"].Melange)
	assert.Equal(t, config.StringList{"custom.melange.yaml", "custom.melange.yaml"}, def.Types["default"].Melange.Bespoke)
}

func TestValidate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		def := &config.ImageDef{
			Name:     "node",
			Upstream: config.Upstream{Package: "nodejs-{{version}}"},
			Types: map[string]config.TypeTemplate{
				"default": {Base: "wolfi-base"},
			},
		}
		require.NoError(t, config.Validate(def))
	})

	t.Run("missing name", func(t *testing.T) {
		def := &config.ImageDef{
			Upstream: config.Upstream{Package: "nodejs-{{version}}"},
			Types:    map[string]config.TypeTemplate{"default": {Base: "wolfi-base"}},
		}
		require.Error(t, config.Validate(def))
	})

	t.Run("missing upstream package", func(t *testing.T) {
		def := &config.ImageDef{
			Name:  "node",
			Types: map[string]config.TypeTemplate{"default": {Base: "wolfi-base"}},
		}
		require.Error(t, config.Validate(def))
	})

	t.Run("no types", func(t *testing.T) {
		def := &config.ImageDef{
			Name:     "node",
			Upstream: config.Upstream{Package: "nodejs-{{version}}"},
		}
		require.Error(t, config.Validate(def))
	})

	t.Run("type missing base", func(t *testing.T) {
		def := &config.ImageDef{
			Name:     "node",
			Upstream: config.Upstream{Package: "nodejs-{{version}}"},
			Types:    map[string]config.TypeTemplate{"default": {}},
		}
		require.Error(t, config.Validate(def))
	})

	t.Run("melange with upstream only", func(t *testing.T) {
		def := &config.ImageDef{
			Name:     "caddy",
			Upstream: config.Upstream{Package: "caddy"},
			Types: map[string]config.TypeTemplate{
				"fips": {
					Base:        "wolfi-base",
					FIPSProfile: config.FIPSProfileGo,
					Environment: map[string]string{"GODEBUG": "fips140=on"},
					Melange: &config.MelangeSpec{
						Upstream: "caddy",
						EnvFile:  "fips.env",
					},
				},
			},
		}
		require.NoError(t, config.Validate(def))
	})

	t.Run("fips type missing profile", func(t *testing.T) {
		def := &config.ImageDef{
			Name:     "node",
			Upstream: config.Upstream{Package: "nodejs-{{version}}"},
			Types: map[string]config.TypeTemplate{
				"fips": {Base: "wolfi-fips"},
			},
		}
		err := config.Validate(def)
		require.ErrorIs(t, err, config.ErrMissingFIPSProfile)
	})

	t.Run("fips type invalid profile", func(t *testing.T) {
		def := &config.ImageDef{
			Name:     "node",
			Upstream: config.Upstream{Package: "nodejs-{{version}}"},
			Types: map[string]config.TypeTemplate{
				"fips": {Base: "wolfi-fips", FIPSProfile: "mystery"},
			},
		}
		err := config.Validate(def)
		require.ErrorIs(t, err, config.ErrInvalidFIPSProfile)
	})

	t.Run("openssl fips requires fips base", func(t *testing.T) {
		def := &config.ImageDef{
			Name:     "node",
			Upstream: config.Upstream{Package: "nodejs-{{version}}"},
			Types: map[string]config.TypeTemplate{
				"fips": {Base: "wolfi-base", FIPSProfile: config.FIPSProfileOpenSSL},
			},
		}
		err := config.Validate(def)
		require.ErrorIs(t, err, config.ErrInvalidFIPSBase)
	})

	t.Run("go fips requires runtime toggle", func(t *testing.T) {
		def := &config.ImageDef{
			Name:     "caddy",
			Upstream: config.Upstream{Package: "caddy"},
			Types: map[string]config.TypeTemplate{
				"fips": {Base: "wolfi-base", FIPSProfile: config.FIPSProfileGo},
			},
		}
		err := config.Validate(def)
		require.ErrorIs(t, err, config.ErrMissingFIPSEnvironment)
	})

	t.Run("melange with bespoke only", func(t *testing.T) {
		def := &config.ImageDef{
			Name:     "custom",
			Upstream: config.Upstream{Package: "custom"},
			Types: map[string]config.TypeTemplate{
				"default": {
					Base: "wolfi-base",
					Melange: &config.MelangeSpec{
						Bespoke: config.StringList{"custom.melange.yaml"},
					},
				},
			},
		}
		require.NoError(t, config.Validate(def))
	})

	t.Run("melange both upstream and bespoke", func(t *testing.T) {
		def := &config.ImageDef{
			Name:     "caddy",
			Upstream: config.Upstream{Package: "caddy"},
			Types: map[string]config.TypeTemplate{
				"fips": {
					Base:        "wolfi-base",
					FIPSProfile: config.FIPSProfileGo,
					Environment: map[string]string{"GODEBUG": "fips140=on"},
					Melange: &config.MelangeSpec{
						Upstream: "caddy",
						Bespoke:  config.StringList{"caddy.melange.yaml"},
					},
				},
			},
		}
		err := config.Validate(def)
		require.ErrorIs(t, err, config.ErrMelangeSourceConflict)
	})

	t.Run("melange empty spec", func(t *testing.T) {
		def := &config.ImageDef{
			Name:     "caddy",
			Upstream: config.Upstream{Package: "caddy"},
			Types: map[string]config.TypeTemplate{
				"fips": {
					Base:        "wolfi-base",
					FIPSProfile: config.FIPSProfileGo,
					Environment: map[string]string{"GODEBUG": "fips140=on"},
					Melange:     &config.MelangeSpec{},
				},
			},
		}
		err := config.Validate(def)
		require.ErrorIs(t, err, config.ErrMelangeNoSource)
	})

	t.Run("melange bespoke with path separator", func(t *testing.T) {
		def := &config.ImageDef{
			Name:     "caddy",
			Upstream: config.Upstream{Package: "caddy"},
			Types: map[string]config.TypeTemplate{
				"fips": {
					Base:        "wolfi-base",
					FIPSProfile: config.FIPSProfileGo,
					Environment: map[string]string{"GODEBUG": "fips140=on"},
					Melange:     &config.MelangeSpec{Bespoke: config.StringList{"../evil/caddy.yaml"}},
				},
			},
		}
		err := config.Validate(def)
		require.ErrorIs(t, err, config.ErrMelangePathTraversal)
	})

	t.Run("melange env-file with path separator", func(t *testing.T) {
		def := &config.ImageDef{
			Name:     "caddy",
			Upstream: config.Upstream{Package: "caddy"},
			Types: map[string]config.TypeTemplate{
				"fips": {
					Base:        "wolfi-base",
					FIPSProfile: config.FIPSProfileGo,
					Environment: map[string]string{"GODEBUG": "fips140=on"},
					Melange: &config.MelangeSpec{
						Upstream: "caddy",
						EnvFile:  "subdir/fips.env",
					},
				},
			},
		}
		err := config.Validate(def)
		require.ErrorIs(t, err, config.ErrMelangePathTraversal)
	})

	t.Run("melange bespoke with backslash", func(t *testing.T) {
		def := &config.ImageDef{
			Name:     "caddy",
			Upstream: config.Upstream{Package: "caddy"},
			Types: map[string]config.TypeTemplate{
				"fips": {
					Base:        "wolfi-base",
					FIPSProfile: config.FIPSProfileGo,
					Environment: map[string]string{"GODEBUG": "fips140=on"},
					Melange:     &config.MelangeSpec{Bespoke: config.StringList{`subdir\caddy.yaml`}},
				},
			},
		}
		err := config.Validate(def)
		require.ErrorIs(t, err, config.ErrMelangePathTraversal)
	})
}
