package ci

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

func TestLoadIntegerGitImpact_usesCommittedRange_andIgnoresDirtyWorktree(t *testing.T) {
	// Given: two committed revisions plus unrelated dirty state.
	repository := t.TempDir()
	runIntegerGit(t, repository, "init", "-q")
	runIntegerGit(t, repository, "config", "user.email", "ci@example.invalid")
	runIntegerGit(t, repository, "config", "user.name", "CI Test")
	writeTestFile(t, filepath.Join(repository, "packages", "upstream.lock.json"), `{"packages":{}}`)
	writeTestFile(t, filepath.Join(repository, "images", "alpha.yaml"), "name: alpha\n")
	runIntegerGit(t, repository, "add", ".")
	runIntegerGit(t, repository, "commit", "-q", "-m", "base")
	base := runIntegerGit(t, repository, "rev-parse", "HEAD")
	writeTestFile(t, filepath.Join(repository, "packages", "upstream.lock.json"), `{"packages":{"alpha":{"file":"alpha.yaml"}}}`)
	writeTestFile(t, filepath.Join(repository, "images", "alpha.yaml"), "name: alpha\ndescription: changed\n")
	runIntegerGit(t, repository, "add", ".")
	runIntegerGit(t, repository, "commit", "-q", "-m", "head")
	head := runIntegerGit(t, repository, "rev-parse", "HEAD")
	writeTestFile(t, filepath.Join(repository, "dirty.txt"), "uncommitted\n")

	// When: exact production impact is loaded from the committed range.
	impact, err := LoadIntegerGitImpact(t.Context(), &IntegerGitImpactOptions{
		Repository: repository, BaseSHA: base, HeadSHA: head, OutputDir: t.TempDir(),
	})

	// Then: only committed paths participate, and base lock/image bytes are
	// materialized for semantic delta planning.
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"images/alpha.yaml", "packages/upstream.lock.json"}, impact.ChangedFiles)
	assert.NotContains(t, impact.ChangedFiles, "dirty.txt")
	lock, err := os.ReadFile(impact.BaseLockPath)
	require.NoError(t, err)
	assert.JSONEq(t, `{"packages":{}}`, string(lock))
	image, err := os.ReadFile(filepath.Join(impact.BaseImagesDir, "alpha.yaml"))
	require.NoError(t, err)
	assert.Equal(t, "name: alpha\n", string(image))
}

func TestLoadIntegerGitImpact_rejectsStaleRevision_andHonorsCancellation(t *testing.T) {
	// Given: a repository and a stale revision.
	repository := t.TempDir()
	runIntegerGit(t, repository, "init", "-q")
	runIntegerGit(t, repository, "config", "user.email", "ci@example.invalid")
	runIntegerGit(t, repository, "config", "user.name", "CI Test")
	writeTestFile(t, filepath.Join(repository, "integer.yaml"), "target: {}\n")
	runIntegerGit(t, repository, "add", ".")
	runIntegerGit(t, repository, "commit", "-q", "-m", "head")
	head := runIntegerGit(t, repository, "rev-parse", "HEAD")

	// When: the base revision does not exist.
	_, err := LoadIntegerGitImpact(t.Context(), &IntegerGitImpactOptions{
		Repository: repository, BaseSHA: strings.Repeat("f", 40), HeadSHA: head, OutputDir: t.TempDir(),
	})

	// Then: stale state fails closed.
	require.ErrorIs(t, err, ErrIntegerBatchPlan)

	// Given: an already-cancelled command context.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// When: Git impact loading starts.
	_, err = LoadIntegerGitImpact(ctx, &IntegerGitImpactOptions{
		Repository: repository, BaseSHA: head, HeadSHA: head, OutputDir: t.TempDir(),
	})

	// Then: cancellation is returned without partial success.
	require.ErrorIs(t, err, context.Canceled)
}

func runIntegerGit(t *testing.T, directory string, arguments ...string) string {
	t.Helper()
	command := exec.CommandContext(t.Context(), "git", arguments...)
	command.Dir = directory
	output, err := command.CombinedOutput()
	require.NoError(t, err, strings.TrimSpace(string(output)))
	return strings.TrimSpace(string(output))
}
