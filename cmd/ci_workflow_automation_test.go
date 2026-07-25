package cmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestCIWorkflowScope_emitsFullScope_whenEventIsPush(t *testing.T) {
	// Given a push event and a writable GitHub output file.
	outputPath := filepath.Join(t.TempDir(), "github-output")
	var stdout bytes.Buffer
	command := newWorkflowTestRoot(&stdout)

	// When the typed scope command runs.
	err := command.Run(context.Background(), []string{
		"verity", "ci", "workflow", "scope",
		"--event-name", "push",
		"--github-output", outputPath,
	})

	// Then every CI surface is enabled without shell path matching.
	require.NoError(t, err)
	data, readErr := os.ReadFile(outputPath)
	require.NoError(t, readErr)
	require.Equal(t, "tests=true\ngo=true\nstatic=true\nnode=true\n", string(data))
}

func TestCIWorkflowScope_preservesExistingChangedPathClasses(t *testing.T) {
	tests := []struct {
		name  string
		paths []string
		want  ciPathScope
	}{
		{name: "Go source", paths: []string{"cmd/check.go"}, want: ciPathScope{tests: true, goChecks: true}},
		{name: "nested Go module", paths: []string{"tools/check/go.mod"}, want: ciPathScope{tests: true, goChecks: true}},
		{name: "Go lint config", paths: []string{".golangci.yml"}, want: ciPathScope{goChecks: true, static: true}},
		{name: "workflow YAML", paths: []string{".github/workflows/release.yaml"}, want: ciPathScope{static: true}},
		{name: "repository script", paths: []string{"scripts/check.sh"}, want: ciPathScope{tests: true, static: true}},
		{name: "site source", paths: []string{"site/src/index.ts"}, want: ciPathScope{node: true}},
		{name: "toolchain config", paths: []string{"mise.toml"}, want: ciPathScope{tests: true, static: true, node: true}},
		{name: "CI workflow", paths: []string{".github/workflows/ci.yaml"}, want: ciPathScope{tests: true, static: true, node: true}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// When the changed paths are classified.
			got := classifyCIPaths(test.paths)

			// Then the legacy selective-job contract is preserved.
			assert.Equal(t, test.want, got)
		})
	}
}

func TestCIWorkflowScope_readsPullRequestChanges_fromGit(t *testing.T) {
	// Given a repository whose pull request changes only site code.
	repository := t.TempDir()
	runGit(t, repository, "init", "-q", "-b", "main")
	runGit(t, repository, "config", "user.name", "CI Test")
	runGit(t, repository, "config", "user.email", "ci@example.invalid")
	baseSHA := commitCIWorkflowFixture(t, repository, "README.txt")
	headSHA := commitCIWorkflowFixture(t, repository, "site/src/index.ts")
	outputPath := filepath.Join(t.TempDir(), "github-output")
	command := newWorkflowTestRoot(&bytes.Buffer{})

	// When the typed scope command computes the base-to-head diff.
	err := command.Run(context.Background(), []string{
		"verity", "ci", "workflow", "scope",
		"--event-name", "pull_request",
		"--repo-root", repository,
		"--base-sha", baseSHA,
		"--head-sha", headSHA,
		"--github-output", outputPath,
	})

	// Then only the Node job is selected.
	require.NoError(t, err)
	data, readErr := os.ReadFile(outputPath)
	require.NoError(t, readErr)
	require.Equal(t, "tests=false\ngo=false\nstatic=false\nnode=true\n", string(data))
}

func TestCIWorkflowCoverage_acceptsProfile_atMinimum(t *testing.T) {
	// Given a valid profile with exactly 80 percent statement coverage.
	profilePath := filepath.Join(t.TempDir(), "coverage.out")
	require.NoError(t, os.WriteFile(profilePath, []byte(
		"mode: set\nexample.go:1.1,2.1 4 1\nexample.go:3.1,4.1 1 0\n",
	), 0o600))
	var stdout bytes.Buffer
	command := newWorkflowTestRoot(&stdout)

	// When the typed coverage gate runs.
	err := command.Run(context.Background(), []string{
		"verity", "ci", "workflow", "coverage",
		"--profile", profilePath,
		"--minimum", "80",
	})

	// Then it reports the measured percentage and passes the boundary.
	require.NoError(t, err)
	require.Equal(t, "Coverage: 80.0%\n", stdout.String())
}

func TestCIWorkflowCoverage_rejectsProfile_belowMinimum(t *testing.T) {
	// Given a valid profile below the required threshold.
	profilePath := filepath.Join(t.TempDir(), "coverage.out")
	require.NoError(t, os.WriteFile(profilePath, []byte(
		"mode: set\nexample.go:1.1,2.1 3 1\nexample.go:3.1,4.1 2 0\n",
	), 0o600))
	command := newWorkflowTestRoot(&bytes.Buffer{})

	// When the typed coverage gate runs.
	err := command.Run(context.Background(), []string{
		"verity", "ci", "workflow", "coverage",
		"--profile", profilePath,
		"--minimum", "80",
	})

	// Then the gate fails with the measured binary outcome.
	require.Error(t, err)
	require.Contains(t, err.Error(), "coverage 60.0% is below required 80.0%")
}

func TestCIAutomationWorkflows_useTypedCommandsAndJobScopedPermissions(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		contains   []string
		notContain []string
	}{
		{
			name: "CI",
			path: filepath.Join("..", ".github", "workflows", "ci.yaml"),
			contains: []string{
				"./verity ci workflow scope",
				"./verity ci workflow coverage",
			},
			notContain: []string{"git diff --name-only", "go tool cover", "bc -l"},
		},
		{
			name: "new issue",
			path: filepath.Join("..", ".github", "workflows", "new-issue.yaml"),
			contains: []string{
				"./verity ci repository-ops parse-image-issue",
				"./verity ci repository-ops add-standalone-image",
			},
			notContain: []string{"parse-image-issue-form.sh", "add-standalone-image.sh"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given the checked-in automation workflow.
			workflow, source := readCIAutomationWorkflow(t, test.path)

			// When its permission and command surfaces are inspected.
			runs := workflowRunCommands(workflow)

			// Then permissions are job-scoped and repository logic is typed Go.
			require.Empty(t, workflow.Permissions)
			for name, job := range workflow.Jobs {
				assert.NotNil(t, job.Permissions, "job %s must declare permissions", name)
			}
			for _, fragment := range test.contains {
				assert.Contains(t, runs, fragment)
			}
			for _, fragment := range test.notContain {
				assert.NotContains(t, source, fragment)
			}
		})
	}
}

type ciAutomationWorkflow struct {
	Permissions map[string]string                  `yaml:"permissions"`
	Jobs        map[string]ciAutomationWorkflowJob `yaml:"jobs"`
}

type ciAutomationWorkflowJob struct {
	Permissions map[string]string `yaml:"permissions"`
	Steps       []struct {
		Run string `yaml:"run"`
	} `yaml:"steps"`
}

func readCIAutomationWorkflow(t *testing.T, path string) (workflow ciAutomationWorkflow, source string) {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, yaml.Unmarshal(data, &workflow))
	return workflow, string(data)
}

func workflowRunCommands(workflow ciAutomationWorkflow) string {
	var runs []string
	for _, job := range workflow.Jobs {
		for _, step := range job.Steps {
			runs = append(runs, step.Run)
		}
	}
	return strings.Join(runs, "\n")
}

func commitCIWorkflowFixture(t *testing.T, repository, name string) string {
	t.Helper()
	path := filepath.Join(repository, filepath.FromSlash(name))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(name), 0o600))
	runGit(t, repository, "add", "--", name)
	runGit(t, repository, "commit", "-q", "-m", name)
	return runGit(t, repository, "rev-parse", "HEAD")
}
