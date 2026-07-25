package command

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"

	"github.com/urfave/cli/v3"

	"github.com/verity-org/verity/internal/ci/workflowops/chartops"
	"github.com/verity-org/verity/internal/ci/workflowops/producer"
)

func newChartMatrixCommand() *cli.Command {
	return &cli.Command{
		Name:  "chart-matrix",
		Usage: "Build the exact chart integration matrix and emit GitHub outputs",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "repo-root", Value: "."},
			&cli.StringFlag{Name: "event-name", Value: "pull_request", Sources: cli.EnvVars("GITHUB_EVENT_NAME")},
			&cli.StringFlag{Name: "base-sha", Sources: cli.EnvVars("BASE_SHA")},
			&cli.StringFlag{Name: "head-sha", Sources: cli.EnvVars("HEAD_SHA")},
			&cli.StringFlag{Name: "input-chart", Sources: cli.EnvVars("INPUT_CHART")},
			&cli.StringFlag{Name: "charts-file", Value: "Chart.yaml"},
			&cli.StringFlag{Name: "verity-config", Value: "verity.yaml"},
			&cli.StringFlag{Name: "values-dir", Value: "test/chart-integration/values"},
			&cli.StringFlag{Name: "github-output", Required: true, Sources: cli.EnvVars("GITHUB_OUTPUT")},
		},
		Action: runChartMatrix,
	}
}

func runChartMatrix(ctx context.Context, command *cli.Command) error {
	result, err := chartops.BuildMatrix(ctx, &chartops.MatrixInput{
		RepoRoot: command.String("repo-root"), EventName: command.String("event-name"),
		BaseSHA: command.String("base-sha"), HeadSHA: command.String("head-sha"), InputChart: command.String("input-chart"),
		ChartsFile: command.String("charts-file"), VerityConfig: command.String("verity-config"), ValuesDir: command.String("values-dir"),
	})
	if err != nil {
		return err
	}
	matrix, err := json.Marshal(result.Charts)
	if err != nil {
		return fmt.Errorf("marshal chart matrix: %w", err)
	}
	if err := appendOutputs(command.String("github-output"), []producer.Output{
		{Name: "matrix", Value: string(matrix)},
		{Name: "strict", Value: strconv.FormatBool(result.Strict)},
	}); err != nil {
		return err
	}
	_, err = fmt.Fprintf(command.Writer, "Discovered charts: %s (strict=%t)\n", matrix, result.Strict)
	return err
}

func newWriteChartSummaryCommand() *cli.Command {
	return &cli.Command{
		Name:  "write-chart-summary",
		Usage: "Render a typed chart shard outcome into the GitHub step summary",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "chart", Required: true, Sources: cli.EnvVars("VERITY_CHART")},
			&cli.StringFlag{Name: "outcome", Required: true, Sources: cli.EnvVars("CHART_TEST_OUTCOME")},
			&cli.StringFlag{Name: "profile", Value: "standard"},
			&cli.StringFlag{Name: "skip-file"},
			&cli.StringFlag{Name: "summary-file", Required: true, Sources: cli.EnvVars("GITHUB_STEP_SUMMARY")},
		},
		Action: runWriteChartSummary,
	}
}

func runWriteChartSummary(_ context.Context, command *cli.Command) (retErr error) {
	summary, err := chartops.BuildSummary(chartops.SummaryInput{
		Chart: command.String("chart"), Outcome: command.String("outcome"),
		Profile: chartops.SummaryProfile(command.String("profile")), SkipFile: command.String("skip-file"),
	})
	if err != nil {
		return err
	}
	closer, writer, err := openAppend(command.String("summary-file"))
	if err != nil {
		return err
	}
	defer func() { retErr = closeWithError(retErr, closer, "GitHub step summary") }()
	if _, err := io.WriteString(writer, summary); err != nil {
		return fmt.Errorf("write GitHub step summary: %w", err)
	}
	return nil
}
