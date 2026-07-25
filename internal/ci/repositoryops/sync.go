package repositoryops

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var (
	ErrStaleWorktree         = errors.New("repository checkout is stale")
	ErrMalformedOutput       = errors.New("repository command returned malformed output")
	ErrPRCreationUnavailable = errors.New("pull request creation is unavailable")
	ErrInvalidSyncRequest    = errors.New("sync request is required")
	ErrInvalidBranch         = errors.New("invalid git branch")
	gitOIDPattern            = regexp.MustCompile(`^[a-f0-9]{40}([a-f0-9]{24})?$`)
	branchPattern            = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._/-]{0,200}$`)
)

type SyncRequestInput struct {
	RepositoryRoot   string
	GitHubRepository string
	BaseBranch       string
	SyncBranch       string
	MaxImages        int
}

type SyncRequest struct {
	repoRoot         string
	githubRepository githubRepository
	baseBranch       string
	syncBranch       string
	maxImages        int
}

func NewSyncRequest(input SyncRequestInput) (SyncRequest, error) {
	root, err := validatedPath("repository root", input.RepositoryRoot)
	if err != nil {
		return SyncRequest{}, err
	}
	repository, err := parseGitHubRepository(input.GitHubRepository)
	if err != nil {
		return SyncRequest{}, err
	}
	base, err := validatedBranch(input.BaseBranch)
	if err != nil {
		return SyncRequest{}, fmt.Errorf("base branch: %w", err)
	}
	branch, err := validatedBranch(input.SyncBranch)
	if err != nil {
		return SyncRequest{}, fmt.Errorf("sync branch: %w", err)
	}
	if input.MaxImages <= 0 {
		return SyncRequest{}, ErrInvalidChangeLimit
	}
	return SyncRequest{repoRoot: root, githubRepository: repository, baseBranch: base, syncBranch: branch, maxImages: input.MaxImages}, nil
}

type SyncService struct {
	Git    GitRunner
	GitHub GitHubRunner
}

type SyncResult struct {
	ChangedFiles   []string
	RestoredFiles  []string
	PullRequestURL string
	Unchanged      bool
}

func (s SyncService) Run(ctx context.Context, request *SyncRequest) (SyncResult, error) {
	if request == nil {
		return SyncResult{}, ErrInvalidSyncRequest
	}
	if s.Git == nil || s.GitHub == nil {
		return SyncResult{}, ErrDependenciesRequired
	}
	repository := syncRepository{git: s.Git, root: request.repoRoot}
	selection, err := repository.selectChanges(ctx, request.maxImages)
	if err != nil {
		return SyncResult{}, err
	}
	if len(selection.Selected) == 0 {
		return SyncResult{Unchanged: true}, nil
	}
	if err := repository.ensureFresh(ctx, request); err != nil {
		return SyncResult{}, err
	}
	result, hasStagedChanges, err := repository.stageChanges(ctx, selection)
	if err != nil {
		return SyncResult{}, err
	}
	if !hasStagedChanges {
		return result, nil
	}
	if err := repository.publish(ctx, request.syncBranch); err != nil {
		return SyncResult{}, err
	}
	pullRequestURL, err := ensureSyncPullRequest(ctx, s.GitHub, request)
	if err != nil {
		return SyncResult{}, err
	}
	result.PullRequestURL = pullRequestURL
	return result, nil
}

func validatedBranch(value string) (string, error) {
	value = strings.TrimSpace(value)
	if !branchPattern.MatchString(value) || strings.Contains(value, "..") || strings.Contains(value, "//") || strings.Contains(value, "@{") || strings.HasSuffix(value, "/") {
		return "", fmt.Errorf("%w: %q", ErrInvalidBranch, value)
	}
	for component := range strings.SplitSeq(value, "/") {
		if strings.HasPrefix(component, ".") || strings.HasSuffix(component, ".") || strings.HasSuffix(component, ".lock") {
			return "", fmt.Errorf("%w: %q", ErrInvalidBranch, value)
		}
	}
	return value, nil
}

func changePaths(changes []FileChange) []string {
	paths := make([]string, len(changes))
	for index, change := range changes {
		paths[index] = change.Path
	}
	return paths
}
