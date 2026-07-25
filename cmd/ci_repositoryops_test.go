package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCIRepositoryOpsCommand_registersScriptReplacementOperations(t *testing.T) {
	// Given
	wanted := []string{
		"patch-image",
		"scan-before",
		"verify-patched",
		"catalog-entry",
		"test-package",
		"verify-sealed-secrets-image",
		"parse-image-issue",
		"add-standalone-image",
		"sync-pr",
	}

	// When
	registered := make(map[string]bool)
	for _, command := range ciRepositoryOpsCommand.Commands {
		registered[command.Name] = true
	}

	// Then
	for _, name := range wanted {
		assert.True(t, registered[name], "missing repository-ops command %q", name)
	}
}

func TestCIRepositoryOpsCommand_isAttachedToCIWithoutEditingLegacyCommandFile(t *testing.T) {
	// When
	registered := false
	for _, command := range CICommand.Commands {
		registered = registered || command.Name == "repository-ops"
	}

	// Then
	assert.True(t, registered)
}

func TestIntegerSyncWorkflow_delegates_pull_request_mutation_to_Go(t *testing.T) {
	// Given
	workflow, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "integer-sync.yaml"))
	require.NoError(t, err)
	text := string(workflow)

	// Then
	assert.Contains(t, text, "./verity ci repository-ops sync-pr")
	assert.Contains(t, text, "permissions: {}")
	assert.NotContains(t, text, "mapfile")
	assert.NotContains(t, text, "git diff --name-only")
	assert.NotContains(t, text, "gh pr view")
	assert.NotContains(t, text, "gh pr create")
}
