package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/ci"
)

func TestCIIntegerBatchPlanCommand_loadsCommittedGitImpact_andIgnoresDirtyFiles(t *testing.T) {
	// Given: an Integer repository with one committed image change and one
	// unrelated dirty image.
	root := setupIntegerBatchGitRepository(t)
	base := runIntegerBatchGit(t, root, "rev-parse", "HEAD")
	writeCommandFixture(t, filepath.Join(root, "images", "alpha.yaml"), integerBatchImage("alpha", "changed"))
	runIntegerBatchGit(t, root, "add", "images/alpha.yaml")
	runIntegerBatchGit(t, root, "commit", "-q", "-m", "head")
	head := runIntegerBatchGit(t, root, "rev-parse", "HEAD")
	writeCommandFixture(t, filepath.Join(root, "images", "dirty.yaml"), integerBatchImage("dirty", "uncommitted"))
	planPath := filepath.Join(root, "out", "plan.json")
	expectedPath := filepath.Join(root, "out", "expected.json")

	// When: the public planner receives only the committed revision range.
	runIntegerBatchCLI(
		t,
		"plan", "--event", "push", "--source-sha", head,
		"--run-id", "42", "--run-attempt", "3", "--publication-id", "integer-publication-42-3", "--batch-id", "42-3",
		"--base-sha", base, "--head-sha", head, "--repo-root", root,
		"--integer-config", filepath.Join(root, "integer.yaml"),
		"--images-dir", filepath.Join(root, "images"), "--apkindex-url", "",
		"--plan-output", planPath, "--expected-output", expectedPath,
	)

	// Then: only the committed image is present in the exact delta.
	data, err := os.ReadFile(planPath)
	require.NoError(t, err)
	plan, err := ci.ParseIntegerBatchPlan(data)
	require.NoError(t, err)
	assert.Equal(t, ci.IntegerBatchModeDelta, plan.Mode)
	require.Len(t, plan.Targets, 1)
	assert.Equal(t, "alpha:latest-default", plan.Targets[0].ID())
}

func TestCIIntegerBatchPlanCommand_rejectsGitHeadSourceMismatch_beforeWritingOutputs(t *testing.T) {
	// Given: a valid committed range but a source identity from another commit.
	root := setupIntegerBatchGitRepository(t)
	base := runIntegerBatchGit(t, root, "rev-parse", "HEAD")
	writeCommandFixture(t, filepath.Join(root, "images", "alpha.yaml"), integerBatchImage("alpha", "changed"))
	runIntegerBatchGit(t, root, "add", "images/alpha.yaml")
	runIntegerBatchGit(t, root, "commit", "-q", "-m", "head")
	head := runIntegerBatchGit(t, root, "rev-parse", "HEAD")
	planPath := filepath.Join(root, "out", "plan.json")

	// When: the claimed source SHA does not identify the planned head commit.
	err := runIntegerBatchCLIErr(
		"plan", "--event", "push", "--source-sha", strings.Repeat("f", 40),
		"--run-id", "42", "--run-attempt", "3", "--publication-id", "integer-publication-42-3", "--batch-id", "42-3",
		"--base-sha", base, "--head-sha", head, "--repo-root", root,
		"--integer-config", filepath.Join(root, "integer.yaml"),
		"--images-dir", filepath.Join(root, "images"), "--apkindex-url", "",
		"--plan-output", planPath, "--expected-output", filepath.Join(root, "out", "expected.json"),
	)

	// Then: planning fails closed before an artifact is created.
	require.ErrorIs(t, err, ci.ErrIntegerBatchPlan)
	assert.NoFileExists(t, planPath)
}

func setupIntegerBatchGitRepository(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	runIntegerBatchGit(t, root, "init", "-q")
	runIntegerBatchGit(t, root, "config", "user.email", "ci@example.invalid")
	runIntegerBatchGit(t, root, "config", "user.name", "CI Test")
	writeCommandFixture(t, filepath.Join(root, "integer.yaml"), "target:\n  registry: ghcr.io/test\n")
	writeCommandFixture(t, filepath.Join(root, "images", "_base", "wolfi-base.yaml"), "# base\n")
	writeCommandFixture(t, filepath.Join(root, "images", "alpha.yaml"), integerBatchImage("alpha", "base"))
	runIntegerBatchGit(t, root, "add", ".")
	runIntegerBatchGit(t, root, "commit", "-q", "-m", "base")
	return root
}

func integerBatchImage(name, description string) string {
	return "name: " + name + "\ndescription: " + description + "\nupstream:\n  package: " + name + "\ntypes:\n  default:\n    base: wolfi-base\n    packages: [\"" + name + "\"]\nversions:\n  latest: {}\n"
}

func runIntegerBatchGit(t *testing.T, directory string, arguments ...string) string {
	t.Helper()
	command := exec.CommandContext(t.Context(), "git", arguments...)
	command.Dir = directory
	output, err := command.CombinedOutput()
	require.NoError(t, err, strings.TrimSpace(string(output)))
	return strings.TrimSpace(string(output))
}
