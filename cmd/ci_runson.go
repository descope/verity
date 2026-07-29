package cmd

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/urfave/cli/v3"

	"github.com/verity-org/verity/internal/ci/runson"
)

func newCIRunsOnCommand() *cli.Command {
	return &cli.Command{
		Name:  "runs-on",
		Usage: "Verify RunsOn runner security and capacity",
		Commands: []*cli.Command{
			{
				Name:  "verify",
				Usage: "Verify the current host against an exact RunsOn canary profile",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "expected-account", Required: true},
					&cli.StringFlag{Name: "expected-region", Required: true},
					&cli.StringFlag{Name: "expected-arch", Required: true},
					&cli.IntFlag{Name: "minimum-cpu", Required: true},
					&cli.IntFlag{Name: "minimum-memory-gib", Required: true},
					&cli.IntFlag{Name: "minimum-disk-gib", Required: true},
				},
				Action: runCIRunsOnVerify,
			},
		},
	}
}

func runCIRunsOnVerify(ctx context.Context, command *cli.Command) error {
	requirements, err := runson.NewRequirements(runson.RequirementInput{
		ExpectedAccount:  command.String("expected-account"),
		ExpectedRegion:   command.String("expected-region"),
		ExpectedArch:     command.String("expected-arch"),
		MinimumCPU:       command.Int("minimum-cpu"),
		MinimumMemoryGiB: command.Int("minimum-memory-gib"),
		MinimumDiskGiB:   command.Int("minimum-disk-gib"),
	})
	if err != nil {
		return fmt.Errorf("parse RunsOn requirements: %w", err)
	}
	report, err := runson.NewVerifier().Verify(ctx, requirements)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(command.Writer)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		return fmt.Errorf("write RunsOn verification report: %w", err)
	}
	return nil
}
