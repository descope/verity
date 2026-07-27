package cmd

import (
	"context"

	"github.com/urfave/cli/v3"
)

func newCIIntegerBatchOutputsCommand() *cli.Command {
	return &cli.Command{
		Name:  "outputs",
		Usage: "Expose validated Integer plan metadata as GitHub outputs",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "plan", Required: true},
			&cli.StringFlag{Name: "github-output", Required: true},
		},
		Action: runCIIntegerBatchOutputs,
	}
}

func runCIIntegerBatchOutputs(_ context.Context, command *cli.Command) error {
	plan, err := readIntegerBatchPlan(command.String("plan"))
	if err != nil {
		return err
	}
	return appendIntegerPlanOutputs(command.String("github-output"), &plan)
}
