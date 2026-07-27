package repositoryops

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

var errCLIGitHubFactory = errors.New("sentinel GitHub factory failure")

func TestCLI_addStandaloneImage_buildsValidatedRequestBeforeGitHubFailure(t *testing.T) {
	// Given
	repositoryRoot := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(repositoryRoot, "copa-config.yaml"), []byte("images: []\n"), 0o600))
	deps := &cliDependencies{
		stdout: &bytes.Buffer{},
		getenv: func(name string) string {
			if name == "GITHUB_REPOSITORY" {
				return "verity-org/verity"
			}
			return "sentinel-token"
		},
		github: func(string) (GitHubRunner, error) { return nil, errCLIGitHubFactory },
	}

	// When
	err := newCLICommand(deps).Run(t.Context(), []string{
		"repository-ops", "add-standalone-image", "--name", "rclone", "--repository", "rclone/rclone",
		"--tag", "v1.70.3", "--issue-number", "123", "--repo-root", repositoryRoot,
	})

	// Then
	require.ErrorIs(t, err, errCLIGitHubFactory)
}

func TestCLI_syncPullRequest_buildsValidatedRequestBeforeGitHubFailure(t *testing.T) {
	// Given
	deps := &cliDependencies{
		stdout: &bytes.Buffer{},
		getenv: func(name string) string {
			if name == "GITHUB_REPOSITORY" {
				return "verity-org/verity"
			}
			return "sentinel-token"
		},
		github: func(string) (GitHubRunner, error) { return nil, errCLIGitHubFactory },
	}

	// When
	err := newCLICommand(deps).Run(t.Context(), []string{
		"repository-ops", "sync-pr", "--repo-root", t.TempDir(), "--base", "main",
		"--branch", "automation/sentinel", "--max-changed-images", "3",
	})

	// Then
	require.ErrorIs(t, err, errCLIGitHubFactory)
}

func TestCLI_syncPullRequest_printsUnchangedSummary(t *testing.T) {
	// Given
	var stdout bytes.Buffer
	gitCalls := 0
	deps := &cliDependencies{
		git: gitRunnerFunc(func(_ context.Context, command GitCommand) (CommandResult, error) {
			gitCalls++
			switch gitCalls {
			case 1:
				require.Equal(t, []string{"diff", "--cached", "--quiet", "--"}, command.Args)
				return CommandResult{}, nil
			case 2:
				require.Equal(t, []string{"status", "--porcelain=v1", "-z", "--untracked-files=all"}, command.Args)
				return CommandResult{}, nil
			default:
				t.Fatalf("unexpected git call %d: %v", gitCalls, command.Args)
				return CommandResult{}, nil
			}
		}),
		github: func(string) (GitHubRunner, error) {
			return gitHubRunnerFunc(func(context.Context, GitHubCommand) (CommandResult, error) {
				t.Fatal("GitHub must not run for an unchanged sync")
				return CommandResult{}, nil
			}), nil
		},
		stdout: &stdout,
		getenv: func(name string) string {
			if name == "GITHUB_REPOSITORY" {
				return "verity-org/verity"
			}
			return "sentinel-token"
		},
	}

	// When
	err := newCLICommand(deps).Run(t.Context(), []string{
		"repository-ops", "sync-pr", "--repo-root", t.TempDir(), "--base", "main",
		"--branch", "automation/sentinel", "--max-changed-images", "3",
	})

	// Then
	require.NoError(t, err)
	require.Equal(t, 2, gitCalls)
	require.Equal(t, "No new package streams\n", stdout.String())
}
