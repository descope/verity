package repositoryops

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"
)

func newAddStandaloneImageCLICommand(deps *cliDependencies) *cli.Command {
	return &cli.Command{
		Name: "add-standalone-image",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "name", Required: true}, &cli.StringFlag{Name: "repository", Required: true},
			&cli.StringFlag{Name: "tag", Required: true}, &cli.StringFlag{Name: "registry", Value: "docker.io"},
			&cli.StringFlag{Name: "issue-number", Required: true}, &cli.StringFlag{Name: "config", Value: "copa-config.yaml"},
			&cli.StringFlag{Name: "repo-root", Value: "."}, &cli.StringFlag{Name: "base", Value: "main"},
		},
		Action: func(ctx context.Context, command *cli.Command) error {
			issue, err := NewImageIssue(ImageIssueInput{
				Name: command.String("name"), Repository: command.String("repository"), Tag: command.String("tag"), Registry: command.String("registry"),
			})
			if err != nil {
				return err
			}
			request, err := NewAddImageRequest(&AddImageRequestInput{
				RepositoryRoot: command.String("repo-root"), GitHubRepository: deps.getenv("GITHUB_REPOSITORY"),
				ConfigPath: command.String("config"), Issue: issue,
				IssueNumber: command.String("issue-number"), BaseBranch: command.String("base"),
			})
			if err != nil {
				return err
			}
			github, err := deps.github(deps.getenv("GITHUB_TOKEN"))
			if err != nil {
				return err
			}
			_, err = (AddImageService{Git: deps.git, GitHub: github}).Run(ctx, &request)
			return err
		},
	}
}

func newSyncPullRequestCLICommand(deps *cliDependencies) *cli.Command {
	return &cli.Command{
		Name: "sync-pr",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "repo-root", Value: "."}, &cli.StringFlag{Name: "base", Value: "main"},
			&cli.StringFlag{Name: "branch", Value: "automation/integer-package-streams"}, &cli.IntFlag{Name: "max-changed-images", Value: 20},
		},
		Action: func(ctx context.Context, command *cli.Command) error {
			request, err := NewSyncRequest(SyncRequestInput{
				RepositoryRoot: command.String("repo-root"), GitHubRepository: deps.getenv("GITHUB_REPOSITORY"),
				BaseBranch: command.String("base"),
				SyncBranch: command.String("branch"), MaxImages: command.Int("max-changed-images"),
			})
			if err != nil {
				return err
			}
			github, err := deps.github(deps.getenv("GITHUB_TOKEN"))
			if err != nil {
				return err
			}
			result, err := (SyncService{Git: deps.git, GitHub: github}).Run(ctx, &request)
			if err == nil && result.Unchanged {
				if _, outputErr := fmt.Fprintln(deps.stdout, "No new package streams"); outputErr != nil {
					return fmt.Errorf("write sync summary: %w", outputErr)
				}
			}
			return err
		},
	}
}
