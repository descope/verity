package cmd

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/urfave/cli/v3"
)

func TestIntegerValidateCommand_SharedBespokePreservesDeclaredVersionMatching(t *testing.T) {
	// Given: an existing shared recipe list whose files each match one declared stream.
	imagesDir, cfgPath := intSetupCmdImages(t)
	intWriteFile(t, filepath.Join(imagesDir, "postgres.yaml"), `
name: postgres
description: postgres
upstream:
  package: postgresql-{{version}}
types:
  default:
    base: wolfi-base
    packages: ["postgresql-{{version}}"]
    melange:
      bespoke: [postgresql-14.yaml, postgresql-15.yaml]
versions:
  "14": {}
  "15": {}
  "16": {}
`)
	bespokeDir := filepath.Join(filepath.Dir(imagesDir), "packages", "bespoke")
	intWriteFile(t, filepath.Join(bespokeDir, "postgresql-14.yaml"), "package:\n  name: postgresql-14\n  version: 14.23\n")
	intWriteFile(t, filepath.Join(bespokeDir, "postgresql-15.yaml"), "package:\n  name: postgresql-15\n  version: 15.18\n")

	// When: the existing unscoped configuration is validated.
	root := &cli.Command{Commands: []*cli.Command{IntegerCommand}}
	err := root.Run(context.Background(), []string{
		"verity", "integer", "validate",
		"--config", cfgPath,
		"--images-dir", imagesDir,
		"--bespoke-dir", bespokeDir,
	})

	// Then: each recipe may match any declared version, preserving prior validation semantics.
	assert.NoError(t, err)
}

func TestIntegerValidateCommand_VersionScopedBespokeOK(t *testing.T) {
	// Given: a bespoke recipe referenced only by version 14 of the default type.
	imagesDir, cfgPath := intSetupCmdImages(t)
	intWriteFile(t, filepath.Join(imagesDir, "postgres.yaml"), `
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
  "16": {}
`)
	bespokeDir := filepath.Join(filepath.Dir(imagesDir), "packages", "bespoke")
	intWriteFile(t, filepath.Join(bespokeDir, "postgresql-14.yaml"), `
package:
  name: postgresql-14
  version: "14.23"
  epoch: 0
`)

	// When: the complete Integer configuration is validated.
	root := &cli.Command{Commands: []*cli.Command{IntegerCommand}}
	err := root.Run(context.Background(), []string{
		"verity", "integer", "validate",
		"--config", cfgPath,
		"--images-dir", imagesDir,
		"--bespoke-dir", bespokeDir,
	})

	// Then: the scoped recipe is recognized as a valid, non-orphan reference.
	assert.NoError(t, err)
}

func TestIntegerValidateCommand_ExactOverrideSuppressesSharedRecipeForVersion(t *testing.T) {
	// Given: version 14 overrides a shared templated recipe while version 15 keeps the shared fallback.
	imagesDir, cfgPath := intSetupCmdImages(t)
	intWriteFile(t, filepath.Join(imagesDir, "postgres.yaml"), `
name: postgres
description: postgres
upstream:
  package: postgresql-{{version}}
types:
  default:
    base: wolfi-base
    packages: ["postgresql-{{version}}"]
    melange:
      bespoke: postgresql-{{version}}.yaml
versions:
  "14":
    melange:
      default:
        bespoke: scoped-postgresql-14.yaml
  "15": {}
`)
	bespokeDir := filepath.Join(filepath.Dir(imagesDir), "packages", "bespoke")
	intWriteFile(t, filepath.Join(bespokeDir, "scoped-postgresql-14.yaml"), "package:\n  name: postgresql-14\n  version: 14.23\n")
	intWriteFile(t, filepath.Join(bespokeDir, "postgresql-15.yaml"), "package:\n  name: postgresql-15\n  version: 15.18\n")

	// When: the complete Integer configuration is validated.
	root := &cli.Command{Commands: []*cli.Command{IntegerCommand}}
	err := root.Run(context.Background(), []string{
		"verity", "integer", "validate",
		"--config", cfgPath,
		"--images-dir", imagesDir,
		"--bespoke-dir", bespokeDir,
	})

	// Then: validation does not require the shadowed shared PostgreSQL 14 recipe.
	assert.NoError(t, err)
}

func TestValidateBespokePackageMismatchIncludesPackagesAndApkoHint(t *testing.T) {
	// Given: a bespoke recipe whose package cannot satisfy the image package list.
	path := filepath.Join(t.TempDir(), "recipe.yaml")
	intWriteFile(t, path, "package:\n  name: other-package\n  version: 1.0.0\n")
	packages := []string{"postgresql-14", "postgresql-14-client"}

	// When: the recipe package is checked against the image packages.
	err := validateBespokePackage(path, packages, func(string) bool { return false })

	// Then: the diagnostic retains the available packages and downstream apko failure hint.
	assert.Error(t, err)
	assert.Contains(t, err.Error(), strings.Join(packages, " "))
	assert.Contains(t, err.Error(), "not in indexes")
}
