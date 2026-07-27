package cmd

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCopaChangeDetection_filters_added_and_modified_images(t *testing.T) {
	repository := t.TempDir()
	runCopaGit(t, repository, "init", "-b", "main")
	runCopaGit(t, repository, "config", "user.email", "test@example.com")
	runCopaGit(t, repository, "config", "user.name", "Test")
	writeCopaConfig(t, repository, `images:
  - name: unchanged
    image: example/unchanged
    tags: {strategy: list, list: ["1"]}
  - name: modified
    image: example/modified
    tags: {strategy: list, list: ["1"]}
`)
	runCopaGit(t, repository, "add", "copa-config.yaml")
	runCopaGit(t, repository, "commit", "-m", "base")
	base := strings.TrimSpace(runCopaGit(t, repository, "rev-parse", "HEAD"))

	writeCopaConfig(t, repository, `images:
  - name: unchanged
    image: example/unchanged
    tags: {strategy: list, list: ["1"]}
  - name: modified
    image: example/modified
    tags: {strategy: list, list: ["2"]}
  - name: added
    image: example/added
    tags: {strategy: list, list: ["1"]}
`)
	runCopaGit(t, repository, "add", "copa-config.yaml")
	runCopaGit(t, repository, "commit", "-m", "head")
	head := strings.TrimSpace(runCopaGit(t, repository, "rev-parse", "HEAD"))

	plan, err := detectCopaChanges(context.Background(), copaChangeRequest{
		repository: repository,
		baseSHA:    base,
		headSHA:    head,
		configPath: "copa-config.yaml",
	})

	require.NoError(t, err)
	assert.Equal(t, copaChangeModeFilter, plan.mode)
	assert.Equal(t, []string{"added", "modified"}, plan.names)
}

func TestCopaChangeDetection_runs_all_when_config_is_unchanged(t *testing.T) {
	repository := t.TempDir()
	runCopaGit(t, repository, "init", "-b", "main")
	runCopaGit(t, repository, "config", "user.email", "test@example.com")
	runCopaGit(t, repository, "config", "user.name", "Test")
	writeCopaConfig(t, repository, "images: []\n")
	runCopaGit(t, repository, "add", "copa-config.yaml")
	runCopaGit(t, repository, "commit", "-m", "base")
	base := strings.TrimSpace(runCopaGit(t, repository, "rev-parse", "HEAD"))
	require.NoError(t, os.WriteFile(filepath.Join(repository, "README.md"), []byte("changed\n"), 0o600))
	runCopaGit(t, repository, "add", "README.md")
	runCopaGit(t, repository, "commit", "-m", "head")
	head := strings.TrimSpace(runCopaGit(t, repository, "rev-parse", "HEAD"))

	plan, err := detectCopaChanges(context.Background(), copaChangeRequest{
		repository: repository,
		baseSHA:    base,
		headSHA:    head,
		configPath: "copa-config.yaml",
	})

	require.NoError(t, err)
	assert.Equal(t, copaChangeModeAll, plan.mode)
	assert.Empty(t, plan.names)
}

func writeCopaConfig(t *testing.T, repository, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(repository, "copa-config.yaml"), []byte(content), 0o600))
}

func runCopaGit(t *testing.T, repository string, arguments ...string) string {
	t.Helper()
	command := exec.CommandContext(t.Context(), "git", arguments...)
	command.Dir = repository
	output, err := command.CombinedOutput()
	require.NoError(t, err, string(output))
	return string(output)
}
