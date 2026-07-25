package repositoryops_test

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ops "github.com/verity-org/verity/internal/ci/repositoryops"
)

const syncHead = "1111111111111111111111111111111111111111"

func TestSyncService_Run_scopesChangesAndBuildsSafeGitHubTranscript(t *testing.T) {
	// Given
	request, err := ops.NewSyncRequest(ops.SyncRequestInput{
		RepositoryRoot:   "/repo",
		GitHubRepository: expectedGitHubRepository,
		BaseBranch:       "main",
		SyncBranch:       "automation/integer-package-streams",
		MaxImages:        1,
	})
	require.NoError(t, err)
	git := successfulSyncGitRunner(t, "?? images/a.yaml\x00 M images/z.yaml\x00")
	github := successfulSyncGitHubRunner(t)

	// When
	result, err := (ops.SyncService{Git: git, GitHub: github}).Run(context.Background(), &request)

	// Then
	require.NoError(t, err)
	assert.Equal(t, []string{"images/a.yaml"}, result.ChangedFiles)
	assert.Equal(t, []string{"images/z.yaml"}, result.RestoredFiles)
	assert.Equal(t, "https://github.com/verity-org/verity/pull/42", result.PullRequestURL)
	gitTranscript := gitCommandTranscript(git.calls)
	assert.Contains(t, gitTranscript, "restore -- images/z.yaml")
	assert.Contains(t, gitTranscript, "add -- images/a.yaml")
	assert.Contains(t, gitTranscript, "push --force-with-lease=refs/heads/automation/integer-package-streams: origin HEAD:refs/heads/automation/integer-package-streams")
	githubTranscript := gitHubCommandTranscript(github.calls)
	assert.Contains(t, githubTranscript, "pr list --head automation/integer-package-streams --base main --state open --json url --jq .[0].url // \"\"")
	assert.Contains(t, githubTranscript, "pr create --base main --head automation/integer-package-streams")
}

func TestSyncService_Run_rejectsDirtyOutOfScopeWorktreeBeforeStaging(t *testing.T) {
	// Given
	request, err := ops.NewSyncRequest(ops.SyncRequestInput{RepositoryRoot: "/repo", GitHubRepository: expectedGitHubRepository, BaseBranch: "main", SyncBranch: "automation/sync", MaxImages: 20})
	require.NoError(t, err)
	git := successfulSyncGitRunner(t, " M .github/workflows/integer-sync.yaml\x00")

	// When
	_, err = (ops.SyncService{Git: git, GitHub: &fakeGitHubRunner{}}).Run(context.Background(), &request)

	// Then
	require.ErrorIs(t, err, ops.ErrDirtyWorktree)
	assert.NotContains(t, gitCommandTranscript(git.calls), "add --")
}

func TestSyncService_Run_rejectsStaleCheckout(t *testing.T) {
	// Given
	request, err := ops.NewSyncRequest(ops.SyncRequestInput{RepositoryRoot: "/repo", GitHubRepository: expectedGitHubRepository, BaseBranch: "main", SyncBranch: "automation/sync", MaxImages: 20})
	require.NoError(t, err)
	git := successfulSyncGitRunner(t, " M images/node.yaml\x00")
	git.run = func(_ context.Context, command ops.GitCommand, _ int) (ops.CommandResult, error) {
		joined := strings.Join(command.Args, " ")
		switch joined {
		case "diff --cached --quiet --":
			return ops.CommandResult{}, nil
		case "status --porcelain=v1 -z --untracked-files=all":
			return ops.CommandResult{Stdout: []byte(" M images/node.yaml\x00")}, nil
		case "rev-parse HEAD":
			return ops.CommandResult{Stdout: []byte(syncHead + "\n")}, nil
		case "ls-remote --exit-code origin refs/heads/main":
			return ops.CommandResult{Stdout: []byte("2222222222222222222222222222222222222222\trefs/heads/main\n")}, nil
		default:
			return ops.CommandResult{}, fmt.Errorf("%w: mutation after stale check %s", errUnexpectedFakeCommand, joined)
		}
	}

	// When
	_, err = (ops.SyncService{Git: git, GitHub: &fakeGitHubRunner{}}).Run(context.Background(), &request)

	// Then
	require.ErrorIs(t, err, ops.ErrStaleWorktree)
	assert.NotContains(t, gitCommandTranscript(git.calls), "add --")
}

func TestParseGitStatus_rejectsPathTraversal(t *testing.T) {
	// When
	_, err := ops.ParseGitStatus([]byte("?? images/../.github/workflows/pwn.yaml\x00"))

	// Then
	require.ErrorIs(t, err, ops.ErrInvalidChangedPath)
}

func TestNewGitHubRunner_requiresGitHubToken(t *testing.T) {
	// When
	_, err := ops.NewGitHubRunner(&fakeCommandRunner{}, "")

	// Then
	require.ErrorIs(t, err, ops.ErrMissingGitHubToken)
}

func TestNewGitHubRunner_passesTokenOnlyThroughEnvironment(t *testing.T) {
	// Given
	commands := &fakeCommandRunner{responses: []ops.CommandResult{{}}}
	github, err := ops.NewGitHubRunner(commands, "secret-token")
	require.NoError(t, err)

	// When
	_, err = github.Run(context.Background(), ops.GitHubCommand{Args: []string{"pr", "list"}, Dir: "/repo"})

	// Then
	require.NoError(t, err)
	require.Len(t, commands.calls, 1)
	assert.NotContains(t, strings.Join(commands.calls[0].Args, " "), "secret-token")
	assert.Equal(t, []string{"GH_TOKEN=secret-token"}, commands.calls[0].Env)
}

func TestSyncService_Run_rejectsMisleadingSuccessfulPRLookupOutput(t *testing.T) {
	// Given
	request, err := ops.NewSyncRequest(ops.SyncRequestInput{RepositoryRoot: "/repo", GitHubRepository: expectedGitHubRepository, BaseBranch: "main", SyncBranch: "automation/sync", MaxImages: 20})
	require.NoError(t, err)
	git := successfulSyncGitRunner(t, " M images/node.yaml\x00")
	github := &fakeGitHubRunner{run: func(_ context.Context, _ ops.GitHubCommand, _ int) (ops.CommandResult, error) {
		return ops.CommandResult{Stdout: []byte("fatal: authentication failed\n")}, nil
	}}

	// When
	_, err = (ops.SyncService{Git: git, GitHub: github}).Run(context.Background(), &request)

	// Then
	require.ErrorIs(t, err, ops.ErrMalformedOutput)
	require.Len(t, github.calls, 1)
}

func TestSyncService_Run_rejectsForgedPullRequestURL(t *testing.T) {
	// Given
	request, err := ops.NewSyncRequest(ops.SyncRequestInput{RepositoryRoot: "/repo", GitHubRepository: expectedGitHubRepository, BaseBranch: "main", SyncBranch: "automation/sync", MaxImages: 20})
	require.NoError(t, err)
	git := successfulSyncGitRunner(t, " M images/node.yaml\x00")
	github := &fakeGitHubRunner{run: func(_ context.Context, _ ops.GitHubCommand, _ int) (ops.CommandResult, error) {
		return ops.CommandResult{Stdout: []byte("https://attacker.example/pull/42\n")}, nil
	}}

	// When
	_, err = (ops.SyncService{Git: git, GitHub: github}).Run(context.Background(), &request)

	// Then
	require.ErrorIs(t, err, ops.ErrMalformedOutput)
}

func TestSyncService_Run_requiresExactExpectedPullRequestURL(t *testing.T) {
	urls := []string{
		"https://github.com/attacker/other/pull/42",
		"http://github.com/verity-org/verity/pull/42",
		"https://GITHUB.com/verity-org/verity/pull/42",
		"https://github.com:443/verity-org/verity/pull/42",
		"https://github.com/verity-org/verity/pull/42/",
		"https://github.com/verity-org/verity/pull/42?view=1",
		"https://github.com/verity-org/verity/pull/42#fragment",
		"https://github.com/%76erity-org/verity/pull/42",
		"https://github.com/verity-org/verity/pull/0",
		"https://github.com/verity-org/verity/pull/01",
		"https://github.com/verity-org/verity/pull/not-a-number",
	}
	for _, pullRequestURL := range urls {
		t.Run(pullRequestURL, func(t *testing.T) {
			// When
			err := runSyncWithPullRequestURL(t, pullRequestURL)

			// Then
			require.ErrorIs(t, err, ops.ErrMalformedOutput)
		})
	}
}

func TestNewSyncRequest_rejectsMissingExpectedGitHubRepository(t *testing.T) {
	// When
	_, err := ops.NewSyncRequest(ops.SyncRequestInput{
		RepositoryRoot: "/repo", BaseBranch: "main", SyncBranch: "automation/sync", MaxImages: 20,
	})

	// Then
	require.ErrorIs(t, err, ops.ErrInvalidGitHubRepository)
}

func TestNewSyncRequest_rejectsGitInvalidBranchNames(t *testing.T) {
	for _, branch := range []string{"automation/sync.lock", "automation/sync."} {
		t.Run(branch, func(t *testing.T) {
			// When
			_, err := ops.NewSyncRequest(ops.SyncRequestInput{
				RepositoryRoot: "/repo", GitHubRepository: expectedGitHubRepository,
				BaseBranch: "main", SyncBranch: branch, MaxImages: 20,
			})

			// Then
			require.ErrorIs(t, err, ops.ErrInvalidBranch)
		})
	}
}

func TestSyncService_Run_reportsUnavailablePRCreationPermission(t *testing.T) {
	// Given
	request, err := ops.NewSyncRequest(ops.SyncRequestInput{RepositoryRoot: "/repo", GitHubRepository: expectedGitHubRepository, BaseBranch: "main", SyncBranch: "automation/sync", MaxImages: 20})
	require.NoError(t, err)
	git := successfulSyncGitRunner(t, " M images/node.yaml\x00")
	github := &fakeGitHubRunner{run: func(_ context.Context, command ops.GitHubCommand, _ int) (ops.CommandResult, error) {
		if command.Args[1] == "list" {
			return ops.CommandResult{}, nil
		}
		return ops.CommandResult{ExitCode: 1, Stderr: []byte("GitHub Actions is not permitted to create or approve pull requests")}, nil
	}}

	// When
	_, err = (ops.SyncService{Git: git, GitHub: github}).Run(context.Background(), &request)

	// Then
	require.ErrorIs(t, err, ops.ErrPRCreationUnavailable)
	assert.Contains(t, err.Error(), "pull-requests:write")
}

func successfulSyncGitRunner(t *testing.T, status string) *fakeGitRunner {
	t.Helper()
	return &fakeGitRunner{run: func(_ context.Context, command ops.GitCommand, _ int) (ops.CommandResult, error) {
		joined := strings.Join(command.Args, " ")
		switch {
		case joined == "diff --cached --quiet --":
			if slices.Contains(command.Args, "--cached") && len(command.Args) == 4 {
				return ops.CommandResult{}, nil
			}
		case joined == "status --porcelain=v1 -z --untracked-files=all":
			return ops.CommandResult{Stdout: []byte(status)}, nil
		case joined == "rev-parse HEAD":
			return ops.CommandResult{Stdout: []byte(syncHead + "\n")}, nil
		case joined == "ls-remote --exit-code origin refs/heads/main":
			return ops.CommandResult{Stdout: []byte(syncHead + "\trefs/heads/main\n")}, nil
		case strings.HasPrefix(joined, "restore -- "):
			return ops.CommandResult{}, nil
		case strings.HasPrefix(joined, "clean -f -- "):
			return ops.CommandResult{}, nil
		case strings.HasPrefix(joined, "add -- "):
			return ops.CommandResult{}, nil
		case joined == "diff --cached --quiet --exit-code":
			return ops.CommandResult{ExitCode: 1}, nil
		case strings.HasPrefix(joined, "config user."):
			return ops.CommandResult{}, nil
		case strings.HasPrefix(joined, "commit -m "):
			return ops.CommandResult{}, nil
		case strings.HasPrefix(joined, "ls-remote --exit-code origin refs/heads/automation/"):
			return ops.CommandResult{ExitCode: 2}, nil
		case strings.HasPrefix(joined, "push --force-with-lease="):
			return ops.CommandResult{}, nil
		}
		return ops.CommandResult{}, fmt.Errorf("%w: git %s", errUnexpectedFakeCommand, joined)
	}}
}

func successfulSyncGitHubRunner(t *testing.T) *fakeGitHubRunner {
	t.Helper()
	return &fakeGitHubRunner{run: func(_ context.Context, command ops.GitHubCommand, _ int) (ops.CommandResult, error) {
		joined := strings.Join(command.Args, " ")
		switch {
		case strings.HasPrefix(joined, "pr list "):
			return ops.CommandResult{}, nil
		case strings.HasPrefix(joined, "pr create "):
			return ops.CommandResult{Stdout: []byte("https://github.com/verity-org/verity/pull/42\n")}, nil
		default:
			return ops.CommandResult{}, fmt.Errorf("%w: GitHub %s", errUnexpectedFakeCommand, joined)
		}
	}}
}

func runSyncWithPullRequestURL(t *testing.T, pullRequestURL string) error {
	t.Helper()
	request, err := ops.NewSyncRequest(ops.SyncRequestInput{
		RepositoryRoot: "/repo", GitHubRepository: expectedGitHubRepository,
		BaseBranch: "main", SyncBranch: "automation/sync", MaxImages: 20,
	})
	require.NoError(t, err)
	git := successfulSyncGitRunner(t, " M images/node.yaml\x00")
	github := &fakeGitHubRunner{run: func(_ context.Context, _ ops.GitHubCommand, _ int) (ops.CommandResult, error) {
		return ops.CommandResult{Stdout: []byte(pullRequestURL + "\n")}, nil
	}}
	_, err = (ops.SyncService{Git: git, GitHub: github}).Run(t.Context(), &request)
	return err
}

func gitCommandTranscript(commands []ops.GitCommand) string {
	lines := make([]string, 0, len(commands))
	for _, command := range commands {
		lines = append(lines, "git "+strings.Join(command.Args, " "))
	}
	return strings.Join(lines, "\n")
}

func gitHubCommandTranscript(commands []ops.GitHubCommand) string {
	lines := make([]string, 0, len(commands))
	for _, command := range commands {
		lines = append(lines, "gh "+strings.Join(command.Args, " "))
	}
	return strings.Join(lines, "\n")
}
