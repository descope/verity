package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/urfave/cli/v3"

	"github.com/verity-org/verity/internal/ci"
)

func newCIIntegerBatchCommand() *cli.Command {
	return &cli.Command{
		Name:  "integer-batch",
		Usage: "Plan and validate exact Integer APK batches",
		Commands: []*cli.Command{
			newCIIntegerBatchPlanCommand(),
			newCIIntegerBatchOutputsCommand(),
			newCIIntegerComponentCommand(),
			newCIIntegerShardCommand(),
			newCIIntegerFinalizeShardCommand(),
			newCIIntegerAggregateCommand(),
		},
	}
}

func readIntegerBatchPlan(path string) (ci.IntegerBatchPlan, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ci.IntegerBatchPlan{}, fmt.Errorf("read Integer batch plan %q: %w", path, err)
	}
	plan, err := ci.ParseIntegerBatchPlan(data)
	if err != nil {
		return ci.IntegerBatchPlan{}, err
	}
	return plan, nil
}

func writeIntegerBatchFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create Integer batch output directory: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write Integer batch output %q: %w", path, err)
	}
	return nil
}
