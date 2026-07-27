package repositoryops_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ops "github.com/verity-org/verity/internal/ci/repositoryops"
)

func TestReadCatalogEntry_buildsSourceAndGoVCSReference(t *testing.T) {
	// Given
	configPath := filepath.Join(t.TempDir(), "copa-config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`images:
  - name: controller
    image: quay.io/example/controller
    goVcsUrl: https://github.com/example/controller
    goVcsTagPrefix: v
    tags:
      strategy: list
      list: [1.2.3]
`), 0o600))
	request, err := ops.NewCatalogRequest(ops.CatalogRequestInput{ConfigPath: configPath, ImageName: "controller", ImageTag: "1.2.3"})
	require.NoError(t, err)

	// When
	entry, err := ops.ReadCatalogEntry(request)

	// Then
	require.NoError(t, err)
	assert.Equal(t, "quay.io/example/controller:1.2.3", entry.Source)
	assert.Equal(t, "https://github.com/example/controller@v1.2.3", entry.GoVCSURL)
}

func TestAddImageService_Run_mutatesConfigAndCreatesPullRequestWithTypedCommands(t *testing.T) {
	// Given
	root := t.TempDir()
	configPath := filepath.Join(root, "copa-config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte("images: []\n"), 0o600))
	issue, err := ops.ParseImageIssue("### Image name\nrclone\n\n### Image repository\nrclone/rclone\n\n### Image tag\nv1.70.3\n\n### Image registry\ndocker.io\n")
	require.NoError(t, err)
	request, err := ops.NewAddImageRequest(&ops.AddImageRequestInput{
		RepositoryRoot: root, GitHubRepository: expectedGitHubRepository,
		ConfigPath:  configPath,
		Issue:       issue,
		IssueNumber: "123",
		BaseBranch:  "main",
	})
	require.NoError(t, err)
	gitMetadata := newFakeGitMetadata(t, root)
	automationCreated := false
	git := &fakeGitRunner{run: func(_ context.Context, command ops.GitCommand, _ int) (ops.CommandResult, error) {
		joined := strings.Join(command.Args, " ")
		if joined == "rev-parse --verify --quiet refs/heads/add-image/rclone" && automationCreated {
			return ops.CommandResult{Stdout: []byte(fakeGitHeadOID + "\n")}, nil
		}
		if result, handled := gitMetadata.response(command); handled {
			return result, nil
		}
		switch {
		case joined == "status --porcelain=v1 -z --untracked-files=all":
			return ops.CommandResult{}, nil
		case strings.HasPrefix(joined, "show-ref --verify --quiet refs/heads/add-image/rclone"):
			return ops.CommandResult{ExitCode: 1}, nil
		case strings.HasPrefix(joined, "config user."), strings.HasPrefix(joined, "checkout -b "), strings.HasPrefix(joined, "add -- "), strings.HasPrefix(joined, "commit -m "), strings.HasPrefix(joined, "push -u origin "):
			if strings.HasPrefix(joined, "checkout -b ") {
				automationCreated = true
			}
			return ops.CommandResult{}, nil
		default:
			return ops.CommandResult{}, fmt.Errorf("%w: git %s", errUnexpectedFakeCommand, joined)
		}
	}}
	github := &fakeGitHubRunner{run: func(_ context.Context, command ops.GitHubCommand, _ int) (ops.CommandResult, error) {
		if strings.HasPrefix(strings.Join(command.Args, " "), "pr create ") {
			return ops.CommandResult{Stdout: []byte("https://github.com/verity-org/verity/pull/123\n")}, nil
		}
		return ops.CommandResult{}, fmt.Errorf("%w: GitHub", errUnexpectedFakeCommand)
	}}

	// When
	result, err := (ops.AddImageService{Git: git, GitHub: github}).Run(context.Background(), &request)

	// Then
	require.NoError(t, err)
	assert.Equal(t, "add-image/rclone", result.Branch)
	assert.Equal(t, "https://github.com/verity-org/verity/pull/123", result.PullRequestURL)
	assert.Contains(t, gitCommandTranscript(git.calls), "git add -- copa-config.yaml")
	assert.Contains(t, gitHubCommandTranscript(github.calls), "gh pr create --title feat: add rclone image")
}

func TestAddImageService_Run_rejectsDirtyWorktreeBeforeConfigMutation(t *testing.T) {
	// Given
	root := t.TempDir()
	configPath := filepath.Join(root, "copa-config.yaml")
	original := []byte("images: []\n")
	require.NoError(t, os.WriteFile(configPath, original, 0o600))
	issue, err := ops.ParseImageIssue("### Image name\nrclone\n\n### Image repository\nrclone/rclone\n\n### Image tag\nv1.70.3\n")
	require.NoError(t, err)
	request, err := ops.NewAddImageRequest(&ops.AddImageRequestInput{RepositoryRoot: root, GitHubRepository: expectedGitHubRepository, ConfigPath: configPath, Issue: issue, IssueNumber: "123", BaseBranch: "main"})
	require.NoError(t, err)
	git := &fakeGitRunner{run: func(_ context.Context, _ ops.GitCommand, _ int) (ops.CommandResult, error) {
		return ops.CommandResult{Stdout: []byte(" M README.md\x00")}, nil
	}}

	// When
	_, err = (ops.AddImageService{Git: git, GitHub: &fakeGitHubRunner{}}).Run(context.Background(), &request)

	// Then
	require.ErrorIs(t, err, ops.ErrDirtyWorktree)
	current, readErr := os.ReadFile(configPath)
	require.NoError(t, readErr)
	assert.Equal(t, original, current)
}

func TestParseImageIssue_preservesFormValidationAndRejectsInjection(t *testing.T) {
	// Given
	body := "### Image name\nrclone\n\n### Image repository\nrclone/rclone; touch /tmp/pwned\n\n### Image tag\nv1.70.3\n\n### Image registry\ndocker.io\n"

	// When
	_, err := ops.ParseImageIssue(body)

	// Then
	require.Error(t, err)
	assert.ErrorIs(t, err, ops.ErrInvalidImageRepository)
}

func TestAppendStandaloneImage_addsPatternEntryWithoutDuplicate(t *testing.T) {
	// Given
	configPath := filepath.Join(t.TempDir(), "copa-config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte("apiVersion: verity.dev/v1\nkind: CopaConfig\nimages: []\n"), 0o600))
	issue, err := ops.ParseImageIssue("### Image name\nrclone\n\n### Image repository\nrclone/rclone\n\n### Image tag\nv1.70.3\n\n### Image registry\n\n")
	require.NoError(t, err)

	// When
	duplicate, err := ops.AppendStandaloneImage(configPath, issue)

	// Then
	require.NoError(t, err)
	assert.False(t, duplicate)
	data, err := os.ReadFile(configPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "image: docker.io/rclone/rclone")
	assert.Contains(t, string(data), "pattern: ^\\d+\\.\\d+\\.\\d+$")
	assert.Contains(t, string(data), "maxTags: 3")
}
