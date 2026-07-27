package command

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/verity-org/verity/internal/ci/workflowops/metrics"
	"github.com/verity-org/verity/internal/ci/workflowops/retry"
)

func newValidateMetricsCommand() *cli.Command {
	return &cli.Command{
		Name:      "validate-metrics-json",
		Usage:     "Validate archived metrics JSON artifacts",
		ArgsUsage: "RUN_ID RUN_ATTEMPT METRICS_DIR",
		Action: func(ctx context.Context, command *cli.Command) error {
			if command.Args().Len() != 3 {
				return fmt.Errorf("%w: validate-metrics-json expects 3 positional arguments", ErrInvalidArguments)
			}
			run, err := expectedRun(command.Args().Get(0), command.Args().Get(1))
			if err != nil {
				return err
			}
			result, err := metrics.ValidateDirectory(ctx, run, command.Args().Get(2))
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(os.Stdout, "Validated %d metrics file(s) for run %d attempt %d\n", result.Count, run.ID(), run.Attempt())
			return err
		},
	}
}

func newArchiveMetricsCommand() *cli.Command {
	return &cli.Command{
		Name:      "archive-metrics",
		Usage:     "Publish validated metrics to the _metrics branch",
		ArgsUsage: "METRICS_DIR RUN_ID RUN_ATTEMPT RUN_CREATED_AT",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "repo-dir", Value: ".", Usage: "Repository containing the origin remote"},
			&cli.IntFlag{Name: "attempts", Value: 256, Sources: cli.EnvVars("METRICS_ARCHIVE_ATTEMPTS")},
			&cli.IntFlag{Name: "retry-delay-seconds", Value: 1, Sources: cli.EnvVars("METRICS_ARCHIVE_RETRY_DELAY_SECONDS")},
			&cli.IntFlag{Name: "retry-jitter-seconds", Value: 3, Sources: cli.EnvVars("METRICS_ARCHIVE_RETRY_JITTER_SECONDS")},
			&cli.DurationFlag{Name: "command-timeout", Value: 2 * time.Minute},
		},
		Action: runArchiveMetrics,
	}
}

func runArchiveMetrics(ctx context.Context, command *cli.Command) error {
	if command.Args().Len() != 4 {
		return fmt.Errorf("%w: archive-metrics expects 4 positional arguments", ErrInvalidArguments)
	}
	run, err := expectedRun(command.Args().Get(1), command.Args().Get(2))
	if err != nil {
		return err
	}
	createdAt, err := time.Parse(time.RFC3339, command.Args().Get(3))
	if err != nil {
		return fmt.Errorf("parse run creation time: %w", err)
	}
	archiver := metrics.Archiver{
		Runner: retry.ExecRunner{},
		Engine: retry.Engine{Sleeper: retry.TimerSleeper{}, Random: retry.SystemRandom{}},
		Stdout: os.Stdout,
	}
	_, err = archiver.Archive(ctx, &metrics.ArchiveOptions{
		RepoDir: command.String("repo-dir"), MetricsDir: command.Args().Get(0), Run: run,
		RunCreatedAt: createdAt, Attempts: command.Int("attempts"),
		RetryDelay:     time.Duration(command.Int("retry-delay-seconds")) * time.Second,
		RetryJitter:    time.Duration(command.Int("retry-jitter-seconds")) * time.Second,
		CommandTimeout: command.Duration("command-timeout"),
	})
	return err
}

func expectedRun(idValue, attemptValue string) (metrics.ExpectedRun, error) {
	id, err := parsePositiveInt64("run id", idValue)
	if err != nil {
		return metrics.ExpectedRun{}, err
	}
	attempt, err := parsePositiveInt64("run attempt", attemptValue)
	if err != nil {
		return metrics.ExpectedRun{}, err
	}
	return metrics.NewExpectedRun(id, attempt)
}
