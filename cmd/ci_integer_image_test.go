package cmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"

	"github.com/verity-org/verity/internal/ci"
)

func TestCIIntegerImagePublishCommand_rejectsMalformedTags_beforeExternalCommands(t *testing.T) {
	// Given: a generated config and malformed final tag input.
	root := t.TempDir()
	configPath := filepath.Join(root, "image.apko.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte("contents:\n  packages: [wolfi-base]\n"), 0o600))
	command := &cli.Command{Commands: []*cli.Command{CICommand}}

	// When: the public Go-owned publication command parses the request.
	err := command.Run(context.Background(), []string{
		"verity", "ci", "integer-image", "publish",
		"--image", "alpha", "--version", "1", "--type", "default",
		"--registry", "ghcr.io/verity-org", "--tags", "1,,latest",
		"--config", configPath, "--workspace", root,
		"--source-sha", strings.Repeat("a", 40), "--run-id", "42", "--run-attempt", "3", "--publication-id", "integer-publication-42-3",
	})

	// Then: malformed input remains classifiable as an exact batch-plan error.
	require.ErrorIs(t, err, ci.ErrIntegerBatchPlan)
}

func TestCIIntegerImageTestPackagesCommand_rejectsUnsupportedArchitecture(t *testing.T) {
	// Given: the public Integer image command.
	command := &cli.Command{Commands: []*cli.Command{CICommand}}

	// When: native package testing receives an unsupported architecture.
	err := command.Run(context.Background(), []string{
		"verity", "ci", "integer-image", "test-packages",
		"--arch", "armv7", "--workspace", t.TempDir(),
	})

	// Then: the typed boundary rejects it before invoking Melange.
	require.ErrorIs(t, err, ci.ErrIntegerBatchPlan)
}
