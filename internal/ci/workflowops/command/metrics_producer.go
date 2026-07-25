package command

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/verity-org/verity/internal/ci/workflowops/githubapi"
	"github.com/verity-org/verity/internal/ci/workflowops/producer"
)

func newResolveMetricsProducerCommand() *cli.Command {
	flags := githubFlags()
	flags = append(
		flags,
		&cli.StringFlag{Name: "run-id", Required: true},
		&cli.StringFlag{Name: "run-attempt", Required: true},
		&cli.StringFlag{Name: "source-sha", Required: true},
		&cli.StringFlag{Name: "artifact-name", Required: true},
		&cli.StringFlag{Name: "github-output", Required: true, Sources: cli.EnvVars("GITHUB_OUTPUT")},
	)
	return &cli.Command{
		Name:   "resolve-metrics-producer",
		Usage:  "Resolve one exact metrics artifact and its protected producer timestamp",
		Flags:  flags,
		Action: runResolveMetricsProducer,
	}
}

func runResolveMetricsProducer(ctx context.Context, command *cli.Command) error {
	github, repository, err := newGitHubClient(command)
	if err != nil {
		return err
	}
	runID, err := parsePositiveInt64("run id", command.String("run-id"))
	if err != nil {
		return err
	}
	runAttempt, err := parsePositiveInt64("run attempt", command.String("run-attempt"))
	if err != nil {
		return err
	}
	artifactName := command.String("artifact-name")
	if !strings.HasPrefix(artifactName, "metrics-") || len(artifactName) == len("metrics-") {
		return fmt.Errorf("%w: exact metrics artifact name is required", ErrInvalidArguments)
	}
	sourceSHA := command.String("source-sha")
	run, err := github.GetWorkflowRunAttempt(ctx, githubapi.GetRunAttemptRequest{
		Repository: repository, RunID: runID, RunAttempt: runAttempt, SourceSHA: sourceSHA,
	})
	if err != nil {
		return fmt.Errorf("resolve exact metrics producer run: %w", err)
	}
	artifact, err := github.GetWorkflowRunArtifact(ctx, githubapi.GetRunArtifactRequest{
		Repository: repository, RunID: runID, ArtifactName: artifactName, SourceSHA: sourceSHA,
	})
	if err != nil {
		return fmt.Errorf("resolve exact metrics producer artifact: %w", err)
	}
	return appendOutputs(command.String("github-output"), []producer.Output{
		{Name: "run-id", Value: strconv.FormatInt(run.ID, 10)},
		{Name: "run-attempt", Value: strconv.FormatInt(run.Attempt, 10)},
		{Name: "artifact-name", Value: artifact.Name},
		{Name: "run-created-at", Value: run.CreatedAt.UTC().Format(time.RFC3339)},
	})
}
