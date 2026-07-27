package command

import (
	"context"

	"github.com/urfave/cli/v3"

	"github.com/verity-org/verity/internal/ci/workflowops/chartresult"
	"github.com/verity-org/verity/internal/ci/workflowops/producer"
)

func newAggregateChartResultsCommand() *cli.Command {
	return &cli.Command{
		Name:  "aggregate-chart-results",
		Usage: "Validate the exact chart integration result set and emit producer identity outputs",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "profile", Value: "standard"},
			&cli.StringSliceFlag{Name: "result", Required: true},
			&cli.StringFlag{Name: "source-sha", Sources: cli.EnvVars("CHART_SOURCE_SHA")},
			&cli.StringFlag{Name: "run-id", Sources: cli.EnvVars("CHART_RUN_ID")},
			&cli.StringFlag{Name: "run-attempt", Sources: cli.EnvVars("CHART_RUN_ATTEMPT")},
			&cli.StringFlag{Name: "publication-id", Sources: cli.EnvVars("CHART_PUBLICATION_ID")},
			&cli.StringFlag{Name: "batch-id", Sources: cli.EnvVars("CHART_BATCH_ID")},
			&cli.StringFlag{Name: "artifact-name", Sources: cli.EnvVars("CHART_ARTIFACT_NAME")},
			&cli.StringFlag{Name: "artifact-digest", Sources: cli.EnvVars("CHART_ARTIFACT_DIGEST")},
			&cli.StringFlag{Name: "github-output", Required: true, Sources: cli.EnvVars("GITHUB_OUTPUT")},
		},
		Action: runAggregateChartResults,
	}
}

func runAggregateChartResults(_ context.Context, command *cli.Command) error {
	result, err := chartresult.Aggregate(&chartresult.Input{
		Profile: command.String("profile"),
		Results: command.StringSlice("result"),
		Identity: chartresult.IdentityInput{
			SourceSHA: command.String("source-sha"), RunID: command.String("run-id"),
			RunAttempt: command.String("run-attempt"), PublicationID: command.String("publication-id"),
			BatchID: command.String("batch-id"), ArtifactName: command.String("artifact-name"),
			ArtifactDigest: command.String("artifact-digest"),
		},
	})
	if err != nil {
		return err
	}
	outputs := result.Outputs()
	workflowOutputs := make([]producer.Output, len(outputs))
	for index, output := range outputs {
		workflowOutputs[index] = producer.Output{Name: output.Name, Value: output.Value}
	}
	return appendOutputs(command.String("github-output"), workflowOutputs)
}
