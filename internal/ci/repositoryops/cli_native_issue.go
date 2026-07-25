package repositoryops

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"
)

var ErrIssueBodyRequired = errors.New("issue body is required")

func newNativePackageCLICommand(deps *cliDependencies) *cli.Command {
	return &cli.Command{
		Name: "test-package",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "kind", Required: true}, &cli.StringFlag{Name: "arch", Required: true},
			&cli.StringFlag{Name: "repo-root", Value: "."},
		},
		Action: func(ctx context.Context, command *cli.Command) error {
			request, err := NewNativePackageRequest(NativePackageInput{Kind: command.String("kind"), Architecture: command.String("arch"), RepositoryRoot: command.String("repo-root")})
			if err != nil {
				return err
			}
			return (NativeService{Commands: deps.commands}).TestPackage(ctx, &request)
		},
	}
}

func newSealedSecretsImageCLICommand(deps *cliDependencies) *cli.Command {
	return &cli.Command{
		Name: "verify-sealed-secrets-image",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "image", Required: true}, &cli.StringFlag{Name: "version", Required: true},
			&cli.StringFlag{Name: "full-version", Required: true}, &cli.StringFlag{Name: "temp-dir"},
		},
		Action: func(ctx context.Context, command *cli.Command) error {
			tempDir := command.String("temp-dir")
			if tempDir == "" {
				tempDir = deps.getenv("RUNNER_TEMP")
			}
			if tempDir == "" {
				tempDir = os.TempDir()
			}
			request, err := NewSealedSecretsImageRequest(SealedSecretsImageInput{
				Image: command.String("image"), Version: command.String("version"), FullVersion: command.String("full-version"), TempDir: tempDir,
			})
			if err != nil {
				return err
			}
			return (NativeService{Commands: deps.commands}).VerifySealedSecretsImage(ctx, request)
		},
	}
}

func newParseImageIssueCLICommand(deps *cliDependencies) *cli.Command {
	return &cli.Command{
		Name:  "parse-image-issue",
		Flags: []cli.Flag{&cli.StringFlag{Name: "body-file"}, &cli.StringFlag{Name: "github-output"}},
		Action: func(_ context.Context, command *cli.Command) error {
			body, err := readCLIIssueBody(command.String("body-file"), deps.getenv("ISSUE_BODY"))
			if err != nil {
				return err
			}
			issue, err := ParseImageIssue(body)
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(deps.stdout, "Parsed: %s → %s:%s\n", issue.Name(), issue.ImageRepository(), issue.Tag()); err != nil {
				return fmt.Errorf("write parsed issue summary: %w", err)
			}
			return appendCLIWorkflowValues(command.String("github-output"), deps.getenv("GITHUB_OUTPUT"), []WorkflowValue{
				{Name: "name", Value: issue.Name()},
				{Name: "repository", Value: issue.Repository()},
				{Name: "tag", Value: issue.Tag()},
				{Name: "registry", Value: issue.Registry()},
			})
		},
	}
}

func readCLIIssueBody(path, environmentBody string) (string, error) {
	if path == "" {
		if environmentBody == "" {
			return "", fmt.Errorf("%w: use --body-file or ISSUE_BODY", ErrIssueBodyRequired)
		}
		return environmentBody, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read issue body %q: %w", path, err)
	}
	return string(data), nil
}
