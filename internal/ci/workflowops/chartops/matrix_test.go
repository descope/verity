package chartops

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildMatrix_selects_changed_chart_dependency_for_pull_request(t *testing.T) {
	// Given a repository where one Chart.yaml dependency changed between exact commits.
	repo := newChartRepository(t, chartFile("1.0.0", "1.0.0"))
	baseSHA := runGitTest(t, repo, "rev-parse", "HEAD")
	require.NoError(t, os.WriteFile(filepath.Join(repo, "Chart.yaml"), []byte(chartFile("2.0.0", "1.0.0")), 0o600))
	runGitTest(t, repo, "add", "Chart.yaml")
	runGitTest(t, repo, "commit", "-m", "update chart")
	headSHA := runGitTest(t, repo, "rev-parse", "HEAD")

	// When the typed chart matrix is built for that pull request range.
	result, err := BuildMatrix(t.Context(), &MatrixInput{
		RepoRoot: repo, EventName: "pull_request", BaseSHA: baseSHA, HeadSHA: headSHA,
	})

	// Then the exact changed chart is selected and strict mode is retained.
	require.NoError(t, err)
	assert.Equal(t, []string{"alpha"}, result.Charts)
	assert.True(t, result.Strict)
}

func TestBuildMatrix_selects_all_charts_for_reusable_producer_run(t *testing.T) {
	// Given a checked-out producer source with two chart dependencies.
	repo := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(repo, "Chart.yaml"), []byte(chartFile("1.0.0", "1.0.0")), 0o600))

	// When the matrix is built outside a pull request.
	result, err := BuildMatrix(t.Context(), &MatrixInput{RepoRoot: repo, EventName: "workflow_call"})

	// Then every declared chart is selected deterministically.
	require.NoError(t, err)
	assert.Equal(t, []string{"alpha", "beta"}, result.Charts)
	assert.False(t, result.Strict)
}

func newChartRepository(t *testing.T, chart string) string {
	t.Helper()
	repo := t.TempDir()
	runGitTest(t, repo, "init")
	runGitTest(t, repo, "config", "user.email", "chart-tests@example.invalid")
	runGitTest(t, repo, "config", "user.name", "Chart Tests")
	require.NoError(t, os.WriteFile(filepath.Join(repo, "Chart.yaml"), []byte(chart), 0o600))
	runGitTest(t, repo, "add", "Chart.yaml")
	runGitTest(t, repo, "commit", "-m", "base chart")
	return repo
}

func runGitTest(t *testing.T, dir string, args ...string) string {
	t.Helper()
	command := exec.CommandContext(t.Context(), "git", append([]string{"-C", dir}, args...)...)
	output, err := command.CombinedOutput()
	require.NoError(t, err, "%s", output)
	return strings.TrimSpace(string(output))
}

func chartFile(alphaVersion, betaVersion string) string {
	return "dependencies:\n" +
		"  - name: alpha\n    version: " + alphaVersion + "\n    repository: https://example.invalid/alpha\n" +
		"  - name: beta\n    version: " + betaVersion + "\n    repository: https://example.invalid/beta\n"
}
