package cmd

import (
	"context"
	"strconv"

	"github.com/urfave/cli/v3"

	"github.com/verity-org/verity/internal/apkrepository"
	"github.com/verity-org/verity/internal/ci/repositoryops"
)

var ciSitePublicationResolvePreviousCommand = &cli.Command{
	Name:  "resolve-previous",
	Usage: "Resolve and verify the latest earlier trusted Build Site publication",
	Flags: []cli.Flag{
		&cli.StringFlag{Name: "repository", Required: true},
		&cli.StringFlag{Name: "workflow", Required: true},
		&cli.StringFlag{Name: "branch", Required: true},
		&cli.StringFlag{Name: "artifact-name", Required: true},
		&cli.Uint64Flag{Name: "before-run-id", Required: true},
		&cli.StringFlag{Name: "github-output", Required: true},
	},
	Action: runCISitePublicationResolvePrevious,
}

func runCISitePublicationResolvePrevious(ctx context.Context, command *cli.Command) error {
	if err := requireSitePublicationArguments(command, 0); err != nil {
		return err
	}
	resolved, err := apkrepository.ResolvePrevious(ctx, &apkrepository.ResolvePreviousOptions{
		Repository: command.String("repository"), Workflow: command.String("workflow"),
		Branch: command.String("branch"), ArtifactName: command.String("artifact-name"),
		BeforeRunID: command.Uint64("before-run-id"),
	})
	if err != nil {
		return err
	}
	return appendPreviousPublicationOutputs(command.String("github-output"), &resolved)
}

func appendPreviousPublicationOutputs(path string, resolved *apkrepository.PreviousPublication) error {
	return repositoryops.AppendWorkflowValues(path, []repositoryops.WorkflowValue{
		{Name: "run_id", Value: strconv.FormatUint(resolved.RunID, 10)},
		{Name: "run_attempt", Value: strconv.FormatUint(resolved.RunAttempt, 10)},
		{Name: "source_sha", Value: resolved.SourceSHA},
		{Name: "artifact_digest", Value: resolved.ArtifactDigest},
		{Name: "manifest_digest", Value: resolved.ManifestDigest},
	})
}
