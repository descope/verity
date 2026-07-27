package repositoryops

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

func ensureSyncPullRequest(ctx context.Context, github GitHubRunner, request *SyncRequest) (string, error) {
	listed, err := runGitHubRequired(ctx, github, GitHubCommand{Dir: request.repoRoot, Args: []string{
		"pr", "list", "--head", request.syncBranch, "--base", request.baseBranch, "--state", "open",
		"--json", "url", "--jq", `.[0].url // ""`,
	}})
	if err != nil {
		return "", err
	}
	if existing := strings.TrimSpace(string(listed.Stdout)); existing != "" {
		return validatedPullRequestURL(existing, request.githubRepository)
	}
	created, err := github.Run(ctx, GitHubCommand{Dir: request.repoRoot, Args: []string{
		"pr", "create", "--base", request.baseBranch, "--head", request.syncBranch,
		"--title", "chore(integer): sync Wolfi package streams",
		"--body", "Automated package-stream additions discovered from Wolfi APKINDEX by `verity integer sync --apply`.",
	}})
	if err != nil {
		return "", fmt.Errorf("create sync pull request: %w", err)
	}
	if created.ExitCode != 0 {
		return "", pullRequestCreationError(created)
	}
	return validatedPullRequestURL(strings.TrimSpace(string(created.Stdout)), request.githubRepository)
}

func runGitHubRequired(ctx context.Context, github GitHubRunner, request GitHubCommand) (CommandResult, error) {
	result, err := github.Run(ctx, request)
	if err != nil {
		return CommandResult{}, fmt.Errorf("run gh: %w", err)
	}
	if result.ExitCode != 0 {
		return CommandResult{}, commandError("gh", result)
	}
	return result, nil
}

func validatedPullRequestURL(value string, repository githubRepository) (string, error) {
	parsed, err := url.ParseRequestURI(value)
	if err != nil {
		return "", fmt.Errorf("%w: pull request URL %q", ErrMalformedOutput, value)
	}
	parts := strings.Split(strings.TrimPrefix(parsed.Path, "/"), "/")
	if parsed.Scheme != "https" || parsed.Host != "github.com" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" ||
		parsed.Opaque != "" || parsed.RawPath != "" || parsed.ForceQuery || len(parts) != 4 || !issueNumberPattern.MatchString(parts[3]) ||
		parsed.Path != repository.pullPath(parts[3]) {
		return "", fmt.Errorf("%w: pull request URL %q", ErrMalformedOutput, value)
	}
	return value, nil
}

func pullRequestCreationError(result CommandResult) error {
	details := strings.TrimSpace(string(append(result.Stdout, result.Stderr...)))
	lower := strings.ToLower(details)
	if strings.Contains(lower, "not permitted to create") || strings.Contains(lower, "resource not accessible") || strings.Contains(lower, "403") {
		return fmt.Errorf("%w: ensure pull-requests:write and repository Actions PR creation are enabled: %s", ErrPRCreationUnavailable, details)
	}
	return commandError("gh", result)
}
