package command

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/verity-org/verity/internal/ci/workflowops/producer"
	"github.com/verity-org/verity/internal/ci/workflowops/retry"
)

func newWaitForWorkflowsCommand() *cli.Command {
	flags := githubFlags()
	flags = append(
		flags,
		&cli.StringFlag{Name: "branch", Value: defaultBranch(), Sources: cli.EnvVars("WAIT_BRANCH")},
		&cli.IntFlag{Name: "lookback-hours", Value: 8, Sources: cli.EnvVars("WAIT_LOOKBACK_HOURS")},
		&cli.IntFlag{Name: "timeout-seconds", Value: 7200, Sources: cli.EnvVars("WAIT_TIMEOUT_SECONDS")},
		&cli.IntFlag{Name: "interval-seconds", Value: 60, Sources: cli.EnvVars("WAIT_INTERVAL_SECONDS")},
		&cli.StringFlag{Name: "event", Sources: cli.EnvVars("WAIT_EVENT")},
		&cli.StringFlag{Name: "batch-id", Sources: cli.EnvVars("WAIT_BATCH_ID")},
		&cli.IntFlag{Name: "expected-runs", Sources: cli.EnvVars("WAIT_EXPECTED_RUNS")},
		&cli.StringSliceFlag{Name: "expected-run", Usage: "Exact WORKFLOW=RUN_ID-RUN_ATTEMPT selector"},
		&cli.StringFlag{Name: "source-sha", Sources: cli.EnvVars("WAIT_SOURCE_SHA", "GITHUB_SHA")},
		&cli.DurationFlag{Name: "api-timeout", Value: 30 * time.Second, Usage: "Per GitHub API attempt timeout"},
		&cli.StringFlag{Name: "github-output", Sources: cli.EnvVars("GITHUB_OUTPUT")},
	)
	return &cli.Command{
		Name:      "wait-for-workflows",
		Usage:     "Wait for exact producer workflow runs",
		ArgsUsage: "WORKFLOW [WORKFLOW ...]",
		Flags:     flags,
		Action:    runWaitForWorkflows,
	}
}

func runWaitForWorkflows(ctx context.Context, command *cli.Command) error {
	if command.Args().Len() == 0 {
		return fmt.Errorf("%w: wait-for-workflows expects at least one workflow", ErrInvalidArguments)
	}
	github, repository, err := newGitHubClient(command)
	if err != nil {
		return err
	}
	var batch *producer.BatchID
	if raw := command.String("batch-id"); raw != "" {
		parsed, err := producer.ParseBatchID(raw)
		if err != nil {
			return err
		}
		batch = &parsed
	}
	exactRuns, err := parseExpectedRuns(command.StringSlice("expected-run"))
	if err != nil {
		return err
	}
	waiter := producer.Waiter{GitHub: github, Clock: systemClock{}, Sleeper: retry.TimerSleeper{}, Stdout: os.Stdout}
	result, err := waiter.Wait(ctx, &producer.Options{
		Repository: repository, Workflows: command.Args().Slice(), Branch: command.String("branch"),
		Lookback: time.Duration(command.Int("lookback-hours")) * time.Hour,
		Timeout:  time.Duration(command.Int("timeout-seconds")) * time.Second,
		Interval: time.Duration(command.Int("interval-seconds")) * time.Second,
		Event:    command.String("event"), Batch: batch, ExpectedRuns: command.Int("expected-runs"),
		ExactRuns: exactRuns, SourceSHA: command.String("source-sha"), APITimeout: command.Duration("api-timeout"),
	})
	if err != nil {
		return err
	}
	return appendOutputs(command.String("github-output"), result.Outputs)
}

func parseExpectedRuns(values []string) ([]producer.ExpectedRun, error) {
	runs := make([]producer.ExpectedRun, 0, len(values))
	for _, value := range values {
		workflow, identity, found := strings.Cut(value, "=")
		if !found {
			return nil, fmt.Errorf("%w: expected run must have form WORKFLOW=RUN_ID-RUN_ATTEMPT", producer.ErrInvalidOptions)
		}
		batch, err := producer.ParseBatchID(identity)
		if err != nil {
			return nil, err
		}
		run, err := producer.NewExpectedRun(workflow, batch.RunID(), batch.Attempt())
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return runs, nil
}

func defaultBranch() string {
	if branch := os.Getenv("GITHUB_REF_NAME"); branch != "" {
		return branch
	}
	return "main"
}
