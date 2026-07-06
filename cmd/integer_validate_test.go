package cmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

func TestIntegerValidateCommand_AllValid(t *testing.T) {
	imagesDir, cfgPath := intSetupCmdImages(t)

	root := &cli.Command{Commands: []*cli.Command{IntegerCommand}}
	err := root.Run(context.Background(), []string{
		"verity", "integer", "validate",
		"--config", cfgPath,
		"--images-dir", imagesDir,
	})
	assert.NoError(t, err)
}

func TestIntegerValidateCommand_InvalidImageYaml(t *testing.T) {
	imagesDir, cfgPath := intSetupCmdImages(t)
	intWriteFile(t, filepath.Join(imagesDir, "broken.yaml"), "not: valid: yaml: [")

	root := &cli.Command{Commands: []*cli.Command{IntegerCommand}}
	err := root.Run(context.Background(), []string{
		"verity", "integer", "validate",
		"--config", cfgPath,
		"--images-dir", imagesDir,
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, errIntegerValidationFailed)
}

func TestIntegerValidateCommand_InvalidConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "integer.yaml")
	intWriteFile(t, cfgPath, ":: bad yaml ::")

	imagesDir := filepath.Join(dir, "images")
	intWriteFile(t, filepath.Join(imagesDir, "node.yaml"), intTestNodeYAML)

	root := &cli.Command{Commands: []*cli.Command{IntegerCommand}}
	err := root.Run(context.Background(), []string{
		"verity", "integer", "validate",
		"--config", cfgPath,
		"--images-dir", imagesDir,
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, errIntegerValidationFailed)
}

func TestIntegerValidateCommand_APKINDEXCheck_Missing(t *testing.T) {
	srv := intMakeAPKINDEXServer(t, "P:curl\nV:8.0.0\n\n")
	imagesDir, cfgPath := intSetupCmdImages(t)

	root := &cli.Command{Commands: []*cli.Command{IntegerCommand}}
	err := root.Run(context.Background(), []string{
		"verity", "integer", "validate",
		"--config", cfgPath,
		"--images-dir", imagesDir,
		"--apkindex-url", srv.URL,
		"--cache-dir", t.TempDir(),
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, errIntegerValidationFailed)
}

func TestIntegerValidateCommand_APKINDEXCheck_Found(t *testing.T) {
	srv := intMakeAPKINDEXServer(t, "P:nodejs-22\nV:22.0.0\n\n")
	imagesDir, cfgPath := intSetupCmdImages(t)

	root := &cli.Command{Commands: []*cli.Command{IntegerCommand}}
	err := root.Run(context.Background(), []string{
		"verity", "integer", "validate",
		"--config", cfgPath,
		"--images-dir", imagesDir,
		"--apkindex-url", srv.URL,
		"--cache-dir", t.TempDir(),
	})
	assert.NoError(t, err)
}

func TestIntegerValidateCommand_SkipsNonYAML(t *testing.T) {
	imagesDir, cfgPath := intSetupCmdImages(t)
	intWriteFile(t, filepath.Join(imagesDir, "README.md"), "# readme")

	root := &cli.Command{Commands: []*cli.Command{IntegerCommand}}
	err := root.Run(context.Background(), []string{
		"verity", "integer", "validate",
		"--config", cfgPath,
		"--images-dir", imagesDir,
	})
	assert.NoError(t, err)
}

// intTestPopeyeImageYAML mirrors the shape of images/popeye.yaml from PR #297.
const intTestPopeyeImageYAML = `
name: popeye
description: "Popeye"
upstream:
  package: popeye
types:
  default:
    base: wolfi-base
    packages: ["popeye"]
    entrypoint: /usr/bin/popeye
    melange:
      bespoke: "popeye.yaml"
versions:
  "0.22.1":
    latest: true
`

const intTestPopeyeBespokeYAML = `
package:
  name: popeye
  version: "0.22.1"
  epoch: 0
`

const intTestMismatchedBespokeYAML = `
package:
  name: not-popeye
  version: "0.22.1"
  epoch: 0
`

const intTestHaproxyImageYAML = `
name: haproxy
description: "HAProxy"
upstream:
  package: haproxy
types:
  default:
    base: wolfi-base
    packages: ["haproxy-{{version}}"]
    entrypoint: /usr/bin/haproxy
    melange:
      bespoke: "haproxy-{{version}}.yaml"
versions:
  "3.0": {}
  "3.1": {}
`

const intTestHaproxy30BespokeYAML = `
package:
  name: haproxy-3.0
  version: "3.0.24"
  epoch: 0
`

const intTestHaproxy31BespokeYAML = `
package:
  name: haproxy-3.1
  version: "3.1.17"
  epoch: 0
`

// TestIntegerValidateCommand_BespokeOK exercises the happy path: an image
// declares melange.bespoke and a matching packages/bespoke/<file>.yaml exists
// whose package.name matches the apko packages: constraint.
func TestIntegerValidateCommand_BespokeOK(t *testing.T) {
	imagesDir, cfgPath := intSetupCmdImages(t)
	intWriteFile(t, filepath.Join(imagesDir, "popeye.yaml"), intTestPopeyeImageYAML)

	bespokeDir := filepath.Join(filepath.Dir(imagesDir), "packages", "bespoke")
	intWriteFile(t, filepath.Join(bespokeDir, "popeye.yaml"), intTestPopeyeBespokeYAML)

	root := &cli.Command{Commands: []*cli.Command{IntegerCommand}}
	err := root.Run(context.Background(), []string{
		"verity", "integer", "validate",
		"--config", cfgPath,
		"--images-dir", imagesDir,
		"--bespoke-dir", bespokeDir,
	})
	assert.NoError(t, err)
}

func TestIntegerValidateCommand_BespokePackageConstraintOK(t *testing.T) {
	imagesDir, cfgPath := intSetupCmdImages(t)
	intWriteFile(t, filepath.Join(imagesDir, "popeye.yaml"), strings.ReplaceAll(intTestPopeyeImageYAML, `packages: ["popeye"]`, `packages: ["popeye=0.22.1-r99"]`))

	bespokeDir := filepath.Join(filepath.Dir(imagesDir), "packages", "bespoke")
	intWriteFile(t, filepath.Join(bespokeDir, "popeye.yaml"), intTestPopeyeBespokeYAML)

	root := &cli.Command{Commands: []*cli.Command{IntegerCommand}}
	err := root.Run(context.Background(), []string{
		"verity", "integer", "validate",
		"--config", cfgPath,
		"--images-dir", imagesDir,
		"--bespoke-dir", bespokeDir,
	})
	assert.NoError(t, err)
}

func TestIntegerValidateCommand_BespokeVersionTemplateOK(t *testing.T) {
	imagesDir, cfgPath := intSetupCmdImages(t)
	intWriteFile(t, filepath.Join(imagesDir, "haproxy.yaml"), intTestHaproxyImageYAML)

	bespokeDir := filepath.Join(filepath.Dir(imagesDir), "packages", "bespoke")
	intWriteFile(t, filepath.Join(bespokeDir, "haproxy-3.0.yaml"), intTestHaproxy30BespokeYAML)
	intWriteFile(t, filepath.Join(bespokeDir, "haproxy-3.1.yaml"), intTestHaproxy31BespokeYAML)

	root := &cli.Command{Commands: []*cli.Command{IntegerCommand}}
	err := root.Run(context.Background(), []string{
		"verity", "integer", "validate",
		"--config", cfgPath,
		"--images-dir", imagesDir,
		"--bespoke-dir", bespokeDir,
	})
	assert.NoError(t, err)
}

// TestIntegerValidateCommand_BespokeMissingFile catches the case where an
// image references a bespoke yaml that doesn't exist on disk.
func TestIntegerValidateCommand_BespokeMissingFile(t *testing.T) {
	imagesDir, cfgPath := intSetupCmdImages(t)
	intWriteFile(t, filepath.Join(imagesDir, "popeye.yaml"), intTestPopeyeImageYAML)

	bespokeDir := filepath.Join(filepath.Dir(imagesDir), "packages", "bespoke")
	require.NoError(t, os.MkdirAll(bespokeDir, 0o755))
	// Note: NOT writing popeye.yaml in the bespoke dir.

	root := &cli.Command{Commands: []*cli.Command{IntegerCommand}}
	err := root.Run(context.Background(), []string{
		"verity", "integer", "validate",
		"--config", cfgPath,
		"--images-dir", imagesDir,
		"--bespoke-dir", bespokeDir,
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, errIntegerValidationFailed)
}

// TestIntegerValidateCommand_BespokeNameMismatch catches the failure mode of
// PR #297: bespoke yaml exists but its package.name doesn't appear in the
// apko packages: constraint, which would surface at apko-publish time as
// "not in indexes".
func TestIntegerValidateCommand_BespokeNameMismatch(t *testing.T) {
	imagesDir, cfgPath := intSetupCmdImages(t)
	intWriteFile(t, filepath.Join(imagesDir, "popeye.yaml"), intTestPopeyeImageYAML)

	bespokeDir := filepath.Join(filepath.Dir(imagesDir), "packages", "bespoke")
	intWriteFile(t, filepath.Join(bespokeDir, "popeye.yaml"), intTestMismatchedBespokeYAML)

	root := &cli.Command{Commands: []*cli.Command{IntegerCommand}}
	err := root.Run(context.Background(), []string{
		"verity", "integer", "validate",
		"--config", cfgPath,
		"--images-dir", imagesDir,
		"--bespoke-dir", bespokeDir,
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, errIntegerValidationFailed)
}

func TestIntegerValidateCommand_BespokeVersionTemplateMismatch(t *testing.T) {
	imagesDir, cfgPath := intSetupCmdImages(t)
	intWriteFile(t, filepath.Join(imagesDir, "haproxy.yaml"), intTestHaproxyImageYAML)

	bespokeDir := filepath.Join(filepath.Dir(imagesDir), "packages", "bespoke")
	intWriteFile(t, filepath.Join(bespokeDir, "haproxy-3.0.yaml"), intTestHaproxy30BespokeYAML)
	intWriteFile(t, filepath.Join(bespokeDir, "haproxy-3.1.yaml"), intTestMismatchedBespokeYAML)

	root := &cli.Command{Commands: []*cli.Command{IntegerCommand}}
	err := root.Run(context.Background(), []string{
		"verity", "integer", "validate",
		"--config", cfgPath,
		"--images-dir", imagesDir,
		"--bespoke-dir", bespokeDir,
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, errIntegerValidationFailed)
}

// TestIntegerValidateCommand_BespokeOrphan flags a bespoke yaml that no image
// references — usually means someone added the bespoke build but forgot to
// wire it into images/.
func TestIntegerValidateCommand_BespokeOrphan(t *testing.T) {
	imagesDir, cfgPath := intSetupCmdImages(t)
	// node.yaml from intSetupCmdImages does NOT use bespoke.

	bespokeDir := filepath.Join(filepath.Dir(imagesDir), "packages", "bespoke")
	intWriteFile(t, filepath.Join(bespokeDir, "stranded.yaml"), intTestPopeyeBespokeYAML)

	root := &cli.Command{Commands: []*cli.Command{IntegerCommand}}
	err := root.Run(context.Background(), []string{
		"verity", "integer", "validate",
		"--config", cfgPath,
		"--images-dir", imagesDir,
		"--bespoke-dir", bespokeDir,
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, errIntegerValidationFailed)
}

// TestIntegerValidateCommand_NoBespokeDir verifies it's fine for a project
// to have no packages/bespoke directory at all (the common case).
func TestIntegerValidateCommand_NoBespokeDir(t *testing.T) {
	imagesDir, cfgPath := intSetupCmdImages(t)
	bespokeDir := filepath.Join(filepath.Dir(imagesDir), "packages", "bespoke")
	// Do not create bespokeDir — node.yaml doesn't reference any bespoke.

	root := &cli.Command{Commands: []*cli.Command{IntegerCommand}}
	err := root.Run(context.Background(), []string{
		"verity", "integer", "validate",
		"--config", cfgPath,
		"--images-dir", imagesDir,
		"--bespoke-dir", bespokeDir,
	})
	assert.NoError(t, err)
}

// TestIntegerValidateCommand_FloatingMajorWithMinorAlias guards the
// happy path of the floating-major fix: when an image declares
// `versions: { "1": {} }` and Wolfi publishes a more-specific minor
// (e.g. "kyverno-1.17"), the renderer aliases declared "1" → "1.17" at
// build time. validate must accept this configuration — the fix would
// be useless if validate then rejected images that rely on it.
func TestIntegerValidateCommand_FloatingMajorWithMinorAlias(t *testing.T) {
	imagesDir, cfgPath := intSetupCmdImages(t)
	intWriteFile(t, filepath.Join(imagesDir, "kyverno.yaml"), `
name: kyverno
description: "Kyverno"
upstream:
  package: "kyverno-{{version}}"
types:
  default:
    base: wolfi-base
    packages: ["kyverno-{{version}}"]
    entrypoint: /usr/bin/kyverno
versions:
  "1": {}
`)
	// Wolfi state at the time of the bug: only kyverno-1.17 exists,
	// no kyverno-1 meta-package. nodejs-22 keeps the existing fixture's
	// `versions: "22"` declaration valid.
	srv := intMakeAPKINDEXServer(t, "P:nodejs-22\nV:22.0.0\n\nP:kyverno-1.17\nV:1.17.5\n\n")

	root := &cli.Command{Commands: []*cli.Command{IntegerCommand}}
	err := root.Run(context.Background(), []string{
		"verity", "integer", "validate",
		"--config", cfgPath,
		"--images-dir", imagesDir,
		"--apkindex-url", srv.URL,
		"--cache-dir", t.TempDir(),
	})
	assert.NoError(t, err, "declared `1` with alias-able minor `1.17` must validate")
}

// TestIntegerValidateCommand_FloatingMajorUnresolvable is the regression
// guard for the validate-time half of the fix. When an image declares
// `versions: { "99": {} }` and Wolfi publishes neither "kyverno-99" nor
// any "kyverno-99.X" minor, validate must FAIL at PR time with a clear
// message — otherwise the configuration slips through and produces a
// nightly Integer Build Image failure instead.
func TestIntegerValidateCommand_FloatingMajorUnresolvable(t *testing.T) {
	imagesDir, cfgPath := intSetupCmdImages(t)
	intWriteFile(t, filepath.Join(imagesDir, "kyverno.yaml"), `
name: kyverno
description: "Kyverno"
upstream:
  package: "kyverno-{{version}}"
types:
  default:
    base: wolfi-base
    packages: ["kyverno-{{version}}"]
    entrypoint: /usr/bin/kyverno
versions:
  "99": {}
`)
	srv := intMakeAPKINDEXServer(t, "P:nodejs-22\nV:22.0.0\n\nP:kyverno-1.17\nV:1.17.5\n\n")

	root := &cli.Command{Commands: []*cli.Command{IntegerCommand}}
	err := root.Run(context.Background(), []string{
		"verity", "integer", "validate",
		"--config", cfgPath,
		"--images-dir", imagesDir,
		"--apkindex-url", srv.URL,
		"--cache-dir", t.TempDir(),
	})
	require.Error(t, err, "declared `99` with no APKINDEX match must fail validate")
	assert.ErrorIs(t, err, errIntegerValidationFailed)
}

// TestIntegerValidateCommand_FloatingMajorUnversionedUpstream is the
// regression for the erlang/haproxy/nginx shape: upstream.package is
// unversioned ("erlang") but type packages template the version
// ("erlang-{{version}}"). Before the VersionedPackagePattern fix, the
// per-version validate guard early-returned because upstream.package
// had no `{{version}}` placeholder — letting `versions: { "99": {} }`
// slip through silently and surface as a nightly Integer Build Image
// failure (`nothing provides "erlang-99"`). With the fix, validate
// uses the type's package pattern and correctly flags the unsatisfiable
// declaration at PR time.
func TestIntegerValidateCommand_FloatingMajorUnversionedUpstream(t *testing.T) {
	imagesDir, cfgPath := intSetupCmdImages(t)
	intWriteFile(t, filepath.Join(imagesDir, "erlang.yaml"), `
name: erlang
description: "Erlang/OTP"
upstream:
  package: erlang
types:
  default:
    base: wolfi-base
    packages: ["erlang-{{version}}"]
    entrypoint: /usr/bin/erl
versions:
  "26": {}
  "99": {}
`)
	// Wolfi has the meta "erlang" and a "erlang-26.3" minor, but no
	// "erlang-26" or "erlang-99" anywhere. Declared "26" is satisfiable
	// (alias resolves to 26.3); "99" is not (validate must flag).
	srv := intMakeAPKINDEXServer(t, "P:nodejs-22\nV:22.0.0\n\nP:erlang\nV:27.0\n\nP:erlang-26.3\nV:26.3.0.0-r0\n\n")

	root := &cli.Command{Commands: []*cli.Command{IntegerCommand}}
	err := root.Run(context.Background(), []string{
		"verity", "integer", "validate",
		"--config", cfgPath,
		"--images-dir", imagesDir,
		"--apkindex-url", srv.URL,
		"--cache-dir", t.TempDir(),
	})
	require.Error(t, err, "declared `99` with type-template-only versioning must still fail validate")
	assert.ErrorIs(t, err, errIntegerValidationFailed)
}

// TestIntegerValidateCommand_BespokeDirMissingButReferenced is a regression
// guard for the double-counting bug flagged by copilot-pull-request-reviewer
// on PR #301: when an image references a bespoke file but the entire bespoke
// directory does not exist on disk, the validator must emit EXACTLY ONE
// failure per affected def — not N per-type ENOENTs from validateBespokeRefs
// PLUS an additional summary failure from reportOrphanBespoke.
//
// The fix detects the missing directory once at startup and routes both
// callers through a single per-def summary path.
func TestIntegerValidateCommand_BespokeDirMissingButReferenced(t *testing.T) {
	imagesDir, cfgPath := intSetupCmdImages(t)
	intWriteFile(t, filepath.Join(imagesDir, "popeye.yaml"), intTestPopeyeImageYAML)

	bespokeDir := filepath.Join(filepath.Dir(imagesDir), "packages", "bespoke")
	// Deliberately do NOT create bespokeDir. popeye.yaml has 1 type that
	// references "popeye.yaml" via melange.bespoke; the buggy old code
	// would emit 1 ENOENT FAIL (per-type) + 1 summary FAIL (orphan) = 2.

	root := &cli.Command{Commands: []*cli.Command{IntegerCommand}}
	err := root.Run(context.Background(), []string{
		"verity", "integer", "validate",
		"--config", cfgPath,
		"--images-dir", imagesDir,
		"--bespoke-dir", bespokeDir,
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, errIntegerValidationFailed)
	assert.Contains(t, err.Error(), "1 error(s)",
		"missing bespoke-dir + N referencing types must produce one summary failure per def, not N+1")
}
