package command

import "github.com/urfave/cli/v3"

func New() *cli.Command {
	return &cli.Command{
		Name:  "workflowops",
		Usage: "Typed workflow report, metrics, retry, wait, and aggregation operations",
		Commands: []*cli.Command{
			newArchiveMetricsCommand(),
			newResolveMetricsProducerCommand(),
			newChartMatrixCommand(),
			newPushReportsCommand(),
			newRetryCommand(),
			newRetryGoBuildCommand(),
			newRetryDockerLoginCommand(),
			newValidateMetricsCommand(),
			newWaitForWorkflowsCommand(),
			newAggregateIntegerCommand(),
			newAggregateChartResultsCommand(),
			newWriteChartSummaryCommand(),
		},
	}
}
