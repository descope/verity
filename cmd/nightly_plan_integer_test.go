package cmd

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"

	intdiscovery "github.com/verity-org/verity/internal/integer/discovery"
)

func TestNightlyPlan_emitsIntegerTarget_whenForced(t *testing.T) {
	// Given an Integer repository with one buildable image.
	root := setupIntegerBatchGitRepository(t)
	outputPath := filepath.Join(t.TempDir(), "integer-matrix.json")
	command := &cli.Command{Commands: []*cli.Command{NightlyCommand}}

	// When the public nightly planner is forced to emit every discovered target.
	err := command.Run(context.Background(), []string{
		"verity", "nightly", "plan",
		"--family", nightlyFamilyInteger,
		"--force",
		"--integer-config", filepath.Join(root, "integer.yaml"),
		"--images-dir", filepath.Join(root, "images"),
		"--apkindex-url", "",
		"--target-registry", "ghcr.io/test",
		"--output", outputPath,
	})
	require.NoError(t, err)

	// Then the emitted matrix contains the declared image and resolved target data.
	data, err := os.ReadFile(outputPath)
	require.NoError(t, err)
	var images []intdiscovery.DiscoveredImage
	require.NoError(t, json.Unmarshal(data, &images))
	require.Len(t, images, 1)
	require.Equal(t, "alpha", images[0].Name)
	require.Equal(t, "latest", images[0].Version)
	require.Equal(t, "default", images[0].Type)
	require.Equal(t, "ghcr.io/test", images[0].Registry)
}
