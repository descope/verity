package repositoryops_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	ops "github.com/verity-org/verity/internal/ci/repositoryops"
)

func TestManualQA_issueToConfig(t *testing.T) {
	// Given
	configPath := filepath.Join(t.TempDir(), "copa-config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte("apiVersion: verity.dev/v1\nkind: CopaConfig\nimages: []\n"), 0o600))
	issue, err := ops.ParseImageIssue("### Image name\nrclone\n\n### Image repository\nrclone/rclone\n\n### Image tag\nv1.70.3\n\n### Image registry\n\n")
	require.NoError(t, err)

	// When
	duplicate, err := ops.AppendStandaloneImage(configPath, issue)

	// Then
	require.NoError(t, err)
	require.False(t, duplicate)
	config, err := os.ReadFile(configPath)
	require.NoError(t, err)
	t.Logf("ISSUE_TO_CONFIG\n%s", config)
}

func TestManualQA_syncDiffTranscript(t *testing.T) {
	// Given
	syncRequest, err := ops.NewSyncRequest(ops.SyncRequestInput{
		RepositoryRoot: "/repo", GitHubRepository: expectedGitHubRepository,
		BaseBranch: "main", SyncBranch: "automation/integer-package-streams", MaxImages: 1,
	})
	require.NoError(t, err)
	git := successfulSyncGitRunner(t, "?? images/a.yaml\x00 M images/z.yaml\x00")
	github := successfulSyncGitHubRunner(t)

	// When
	result, err := (ops.SyncService{Git: git, GitHub: github}).Run(context.Background(), &syncRequest)

	// Then
	require.NoError(t, err)
	t.Logf("SYNC_RESULT changed=%v restored=%v pr=%s", result.ChangedFiles, result.RestoredFiles, result.PullRequestURL)
	t.Logf("GIT_TRANSCRIPT\n%s", gitCommandTranscript(git.calls))
	t.Logf("GH_TRANSCRIPT\n%s", gitHubCommandTranscript(github.calls))
}

func TestManualQA_maliciousInputRejected(t *testing.T) {
	// When
	_, maliciousErr := ops.ParseImageIssue("### Image name\nrclone\n\n### Image repository\nrclone/rclone; touch /tmp/pwned\n\n### Image tag\nv1.70.3\n")

	// Then
	require.ErrorIs(t, maliciousErr, ops.ErrInvalidImageRepository)
	t.Logf("MALICIOUS_INPUT_REJECTED %v", maliciousErr)
}
