package repositoryops_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"

	ops "github.com/verity-org/verity/internal/ci/repositoryops"
)

func TestCLI_syncPR_requiresGITHUBTokenAndDoesNotUseGHTokenFallback(t *testing.T) {
	// Given
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "must-not-be-used")
	t.Setenv("GITHUB_REPOSITORY", expectedGitHubRepository)
	root := &cli.Command{Commands: []*cli.Command{ops.NewCLICommand()}}

	// When
	err := root.Run(context.Background(), []string{"verity", "repository-ops", "sync-pr", "--repo-root", t.TempDir()})

	// Then
	require.ErrorIs(t, err, ops.ErrMissingGitHubToken)
}
