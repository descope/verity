package repositoryops

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

var (
	ErrInvalidIssueNumber    = errors.New("invalid issue number")
	ErrStaleAutomationBranch = errors.New("automation branch already exists locally")
	ErrRepositoryRoot        = errors.New("repository root is unavailable")
	ErrCatalogOutsideRoot    = errors.New("catalog path is outside repository root")
	issueNumberPattern       = regexp.MustCompile(`^[1-9]\d{0,18}$`)
)

type AddImageRequestInput struct {
	RepositoryRoot   string
	GitHubRepository string
	ConfigPath       string
	Issue            ImageIssue
	IssueNumber      string
	BaseBranch       string
}

type AddImageRequest struct {
	repoRoot         string
	githubRepository githubRepository
	configPath       string
	configRelative   string
	issue            ImageIssue
	issueNumber      string
	baseBranch       string
	branch           string
}

func NewAddImageRequest(input *AddImageRequestInput) (AddImageRequest, error) {
	if input == nil {
		return AddImageRequest{}, fmt.Errorf("%w: add-image input is required", ErrInvalidImageIssue)
	}
	root, err := filepath.Abs(input.RepositoryRoot)
	if err != nil {
		return AddImageRequest{}, fmt.Errorf("resolve repository root: %w", err)
	}
	rootInfo, err := os.Stat(root)
	if err != nil || !rootInfo.IsDir() {
		return AddImageRequest{}, fmt.Errorf("%w: %q", ErrRepositoryRoot, root)
	}
	repository, err := parseGitHubRepository(input.GitHubRepository)
	if err != nil {
		return AddImageRequest{}, err
	}
	configCandidate := input.ConfigPath
	if !filepath.IsAbs(configCandidate) {
		configCandidate = filepath.Join(root, configCandidate)
	}
	configPath, err := filepath.Abs(configCandidate)
	if err != nil {
		return AddImageRequest{}, fmt.Errorf("resolve catalog path: %w", err)
	}
	relative, err := filepath.Rel(root, configPath)
	if err != nil || relative == "." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return AddImageRequest{}, ErrCatalogOutsideRoot
	}
	issueNumber := strings.TrimSpace(input.IssueNumber)
	if !issueNumberPattern.MatchString(issueNumber) {
		return AddImageRequest{}, fmt.Errorf("%w: %q", ErrInvalidIssueNumber, input.IssueNumber)
	}
	baseBranch, err := validatedBranch(input.BaseBranch)
	if err != nil {
		return AddImageRequest{}, fmt.Errorf("base branch: %w", err)
	}
	if input.Issue.name == "" {
		return AddImageRequest{}, ErrInvalidImageIssue
	}
	branch := "add-image/" + branchComponent(input.Issue.name)
	return AddImageRequest{
		repoRoot: root, githubRepository: repository, configPath: configPath, configRelative: filepath.ToSlash(relative),
		issue: input.Issue, issueNumber: issueNumber, baseBranch: baseBranch, branch: branch,
	}, nil
}

type AddImageService struct {
	Git    GitRunner
	GitHub GitHubRunner
}

type AddImageResult struct {
	Duplicate      bool
	Branch         string
	PullRequestURL string
}

func (s AddImageService) Run(ctx context.Context, request *AddImageRequest) (AddImageResult, error) {
	if request == nil {
		return AddImageResult{}, fmt.Errorf("%w: add-image request is required", ErrInvalidImageIssue)
	}
	if s.Git == nil || s.GitHub == nil {
		return AddImageResult{}, ErrDependenciesRequired
	}
	status, err := runGitRequired(ctx, s.Git, GitCommand{
		Dir: request.repoRoot, Args: []string{"status", "--porcelain=v1", "-z", "--untracked-files=all"},
	})
	if err != nil {
		return AddImageResult{}, err
	}
	changes, err := ParseGitStatus(status.Stdout)
	if err != nil {
		return AddImageResult{}, err
	}
	if len(changes) != 0 {
		return AddImageResult{}, fmt.Errorf("%w: repository must be clean before adding an image", ErrDirtyWorktree)
	}
	branchStatus, err := (syncRepository{git: s.Git, root: request.repoRoot}).exit(ctx, []string{
		"show-ref", "--verify", "--quiet", "refs/heads/" + request.branch,
	}, 0, 1)
	if err != nil {
		return AddImageResult{}, err
	}
	if branchStatus == 0 {
		return AddImageResult{}, fmt.Errorf("%w: %s", ErrStaleAutomationBranch, request.branch)
	}
	snapshot, err := captureAddImageTransaction(ctx, s.Git, request)
	if err != nil {
		return AddImageResult{}, err
	}
	result, err := s.apply(ctx, request, snapshot.git)
	if err == nil {
		return result, nil
	}
	if rollbackErr := snapshot.restore(ctx, s.Git); rollbackErr != nil {
		return AddImageResult{}, errors.Join(err, fmt.Errorf("%w: %w", ErrWorktreeRollback, rollbackErr))
	}
	return AddImageResult{}, err
}

func (s AddImageService) apply(
	ctx context.Context,
	request *AddImageRequest,
	snapshot *gitRepositorySnapshot,
) (AddImageResult, error) {
	duplicate, err := AppendStandaloneImage(request.configPath, request.issue)
	if err != nil {
		return AddImageResult{}, err
	}
	if duplicate {
		if err := closeDuplicateIssue(ctx, s.GitHub, request); err != nil {
			return AddImageResult{}, err
		}
		return AddImageResult{Duplicate: true}, nil
	}
	if err := snapshot.commitStandaloneImage(ctx, s.Git, request); err != nil {
		return AddImageResult{}, err
	}
	pullRequestURL, err := createStandaloneImagePullRequest(ctx, s.GitHub, request)
	if err != nil {
		return AddImageResult{}, err
	}
	return AddImageResult{Branch: request.branch, PullRequestURL: pullRequestURL}, nil
}

func closeDuplicateIssue(ctx context.Context, github GitHubRunner, request *AddImageRequest) error {
	body := "Image **" + request.issue.name + "** already exists in copa-config.yaml. Closing as duplicate."
	commands := [][]string{
		{"issue", "comment", request.issueNumber, "--body", body},
		{"issue", "close", request.issueNumber},
	}
	for _, args := range commands {
		if _, err := runGitHubRequired(ctx, github, GitHubCommand{Dir: request.repoRoot, Args: args}); err != nil {
			return err
		}
	}
	return nil
}

func (snapshot *gitRepositorySnapshot) commitStandaloneImage(
	ctx context.Context,
	git GitRunner,
	request *AddImageRequest,
) error {
	message := fmt.Sprintf(
		"feat: add %s image\n\nAdds %s:%s to copa-config.yaml.\n\nCopa will patch this image on the next scan-and-patch workflow run.\n\nCloses #%s",
		request.issue.name, request.issue.ImageRepository(), request.issue.tag, request.issueNumber,
	)
	commands := [][]string{
		{"config", "user.name", "github-actions[bot]"},
		{"config", "user.email", "github-actions[bot]@users.noreply.github.com"},
		{"checkout", "-b", request.branch},
		{"add", "--", request.configRelative},
		{"commit", "-m", message},
	}
	for _, args := range commands {
		if _, err := runGitRequired(ctx, git, GitCommand{Dir: request.repoRoot, Args: args}); err != nil {
			return err
		}
	}
	if err := snapshot.remote.prepare(ctx, git, snapshot.automationRef); err != nil {
		return err
	}
	if _, err := runGitRequired(ctx, git, GitCommand{
		Dir: request.repoRoot, Args: []string{"push", "-u", addImageRemote, request.branch},
	}); err != nil {
		return err
	}
	return nil
}

func createStandaloneImagePullRequest(ctx context.Context, github GitHubRunner, request *AddImageRequest) (string, error) {
	title := "feat: add " + request.issue.name + " image"
	body := fmt.Sprintf(
		"Adds `%s` to `copa-config.yaml` (requested version: `%s`).\n\n"+
			"The entry uses a semver pattern strategy (`^\\d+\\.\\d+\\.\\d+$`, maxTags: 3) so verity continuously tracks and patches the latest releases — not just the requested tag.\n\n"+
			"## What happens next\n\n1. This image is added to `copa-config.yaml` under `images:`\n"+
			"2. **scan-and-patch workflow** will patch and publish matching tags to GHCR\n\nCloses #%s",
		request.issue.ImageRepository(), request.issue.tag, request.issueNumber,
	)
	result, err := github.Run(ctx, GitHubCommand{Dir: request.repoRoot, Args: []string{
		"pr", "create", "--title", title, "--body", body, "--label", "new-image", "--base", request.baseBranch, "--head", request.branch,
	}})
	if err != nil {
		return "", fmt.Errorf("create standalone image pull request: %w", err)
	}
	if result.ExitCode != 0 {
		return "", pullRequestCreationError(result)
	}
	return validatedPullRequestURL(strings.TrimSpace(string(result.Stdout)), request.githubRepository)
}

func branchComponent(value string) string {
	var builder strings.Builder
	lastHyphen := false
	for _, character := range value {
		allowed := unicode.IsLetter(character) || unicode.IsDigit(character) || character == '-'
		if allowed {
			builder.WriteRune(character)
			lastHyphen = character == '-'
			continue
		}
		if !lastHyphen {
			builder.WriteByte('-')
			lastHyphen = true
		}
	}
	return strings.Trim(builder.String(), "-")
}
