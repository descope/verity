package cmd

import (
	"context"
	"os"
	"path/filepath"
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
