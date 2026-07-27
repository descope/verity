package command

import (
	"context"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"

	"github.com/verity-org/verity/internal/ci/workflowops/githubapi"
	workflowinteger "github.com/verity-org/verity/internal/ci/workflowops/integer"
)

func newAggregateIntegerCommand() *cli.Command {
	return &cli.Command{
		Name:      "aggregate-integer-results",
		Usage:     "Aggregate exact Integer child reports",
		ArgsUsage: "EXPECTED_JSON RESULTS_DIR CHILD_RESULT REPOSITORY RUN_ID BATCH_ID",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "github-step-summary", Sources: cli.EnvVars("GITHUB_STEP_SUMMARY")},
			&cli.StringFlag{Name: "source-sha", Sources: cli.EnvVars("INTEGER_SOURCE_SHA", "GITHUB_SHA")},
		},
		Action: runAggregateInteger,
	}
}

func runAggregateInteger(ctx context.Context, command *cli.Command) error {
	if command.Args().Len() != 6 {
		return fmt.Errorf("%w: aggregate-integer-results expects 6 positional arguments", ErrInvalidArguments)
	}
	childResult, err := workflowinteger.ParseChildResult(command.Args().Get(2))
	if err != nil {
		return err
	}
	repository, err := githubapi.NewRepository(command.Args().Get(3))
	if err != nil {
		return err
	}
	runID, err := parsePositiveInt64("run id", command.Args().Get(4))
	if err != nil {
		return err
	}
	file, summary, err := openAppend(command.String("github-step-summary"))
	if err != nil {
		return err
	}
	aggregator := workflowinteger.Aggregator{Stdout: os.Stdout, Stderr: os.Stderr, Summary: summary}
	_, aggregateErr := aggregator.Aggregate(ctx, &workflowinteger.Options{
		ExpectedPath: command.Args().Get(0), ResultsDir: command.Args().Get(1), ChildResult: childResult,
		Repository: repository, RunID: runID, BatchID: command.Args().Get(5), SourceSHA: command.String("source-sha"),
	})
	return closeWithError(aggregateErr, file, "GitHub step summary")
}
