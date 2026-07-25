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

func TestCIAPKRepositoryCommand_assembles_guarded_empty_repository(t *testing.T) {
	// Given an empty package source and the public CI command tree.
	root := t.TempDir()
	source := filepath.Join(root, "source")
	output := filepath.Join(root, "output")
	require.NoError(t, os.MkdirAll(source, 0o755))
	command := &cli.Command{Commands: []*cli.Command{CICommand}}

	// When the nested assemble command runs.
	err := command.Run(context.Background(), []string{
		"verity", "ci", "apk-repository", "assemble", "--output", output, source,
	})

	// Then the Go command owns the repository behavior end-to-end.
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(output, ".no-apks-found"))
}

func TestCIAPKRepositoryCommand_registers_all_publication_operations(t *testing.T) {
	// Given the CI command tree.
	wanted := []string{"assemble", "snapshot", "delta", "validate", "download-approved", "restore-previous", "select"}

	// When the APK repository subcommands are enumerated.
	registered := make(map[string]bool)
	for _, command := range ciAPKRepositoryCommand.Commands {
		registered[command.Name] = true
	}

	// Then every former shell-script responsibility has a typed command.
	for _, name := range wanted {
		assert.True(t, registered[name], "missing apk-repository command %q", name)
	}
}
