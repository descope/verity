package cmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"

	"github.com/verity-org/verity/internal/integer/melange"
)

func TestIntegerMelangePrepareCommandResolvesSpecAndWritesGitHubOutput(t *testing.T) {
	// Given: a local image variant with a bespoke package recipe.
	rootDir := t.TempDir()
	writeIntegerMelangeImage(t, rootDir, "caddy", `
name: caddy
description: caddy
upstream:
  package: caddy
types:
  fips:
    base: wolfi-base
    fips-profile: go
    packages: ["caddy"]
    environment:
      GODEBUG: "fips140=on"
    melange:
      upstream: caddy
      env-file: fips.env
versions:
  "2": {}
`)
	outputPath := filepath.Join(rootDir, "github-output")
	var captured melange.BuildOptions
	originalPrepare := integerMelangePrepare
	integerMelangePrepare = func(_ context.Context, options *melange.BuildOptions) error {
		captured = *options
		return nil
	}
	t.Cleanup(func() { integerMelangePrepare = originalPrepare })
	root := &cli.Command{Commands: []*cli.Command{IntegerCommand}}

	// When: preparation is requested through the public CLI.
	err := root.Run(context.Background(), []string{
		"verity", "integer", "melange", "prepare",
		"--root", rootDir,
		"--image", "caddy",
		"--version", "2",
		"--type", "fips",
		"--github-output", outputPath,
	})

	// Then: the local spec is passed to the Go implementation and workflow outputs are emitted.
	require.NoError(t, err)
	assert.Equal(t, rootDir, captured.Paths.Root)
	assert.Equal(t, melange.Spec{Upstream: "caddy", EnvFile: "fips.env"}, captured.Spec)
	data, readErr := os.ReadFile(outputPath)
	require.NoError(t, readErr)
	assert.Equal(t, "needed=true\nenv_file=fips.env\nbuild_option=\n", string(data))
}

func TestIntegerMelangeBuildCommandPreservesStagedBuildOptions(t *testing.T) {
	// Given: a versioned local package recipe and an injectable Go build boundary.
	rootDir := t.TempDir()
	writeIntegerMelangeImage(t, rootDir, "cilium", `
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
versions:
  "1.19": {}
`)
	var captured melange.BuildOptions
	originalBuild := integerMelangeBuild
	integerMelangeBuild = func(_ context.Context, options *melange.BuildOptions) error {
		captured = *options
		return nil
	}
	t.Cleanup(func() { integerMelangeBuild = originalBuild })
	root := &cli.Command{Commands: []*cli.Command{IntegerCommand}}

	// When: a downloaded staged build is requested for the native architecture.
	err := root.Run(context.Background(), []string{
		"verity", "integer", "melange", "build",
		"--root", rootDir,
		"--image", "cilium",
		"--version", "1.19",
		"--type", "default",
		"--arch", "aarch64",
		"--staged",
	})

	// Then: version substitution and staged state reach the Go implementation unchanged.
	require.NoError(t, err)
	assert.Equal(t, rootDir, captured.Paths.Root)
	assert.Equal(t, melange.Spec{Upstream: "cilium-1.19"}, captured.Spec)
	assert.Equal(t, melange.ArchitectureAArch64, captured.Arch)
	assert.True(t, captured.Staged)
}

func writeIntegerMelangeImage(t *testing.T, rootDir, image, body string) {
	t.Helper()
	path := filepath.Join(rootDir, "images", image+".yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}
