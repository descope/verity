package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"

	"github.com/verity-org/verity/internal/chartgen"
)

// ChartGenCommand generates patched wrapper Helm charts for each chart in
// Chart.yaml, pointing image values at patched images in the target registry.
var ChartGenCommand = &cli.Command{
	Name:  "chart-gen",
	Usage: "Generate and push patched wrapper Helm charts from Chart.yaml",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "charts-file",
			Usage: "Helm Chart.yaml whose dependencies: provides chart specs",
			Value: "Chart.yaml",
		},
		&cli.StringFlag{
			Name:  "verity-config",
			Usage: "Path to verity.yaml (tag variant overrides and value path hints)",
			Value: "verity.yaml",
		},
		&cli.StringFlag{
			Name:     "target-registry",
			Usage:    "Registry where patched images live (e.g., ghcr.io/verity-org)",
			Required: true,
		},
		&cli.StringFlag{
			Name:     "chart-registry",
			Usage:    "OCI registry to push wrapper charts (e.g., oci://ghcr.io/verity-org/charts)",
			Required: true,
		},
		&cli.StringFlag{
			Name:  "exclude-names",
			Usage: "Comma-separated image names to exclude (e.g., Integer/Wolfi image names)",
		},
		&cli.BoolFlag{
			Name:  "dry-run",
			Usage: "Output JSON plan without pushing charts",
		},
		&cli.BoolFlag{
			Name:  "strict",
			Usage: "Fail (exit non-zero) if any chart in the charts file produces zero patched image mappings — surfaces silent config gaps (e.g., chart added without a matching replacements: entry)",
		},
	},
	Action: func(_ context.Context, cmd *cli.Command) error {
		cfg := &chartgen.Config{
			ChartsFile:     cmd.String("charts-file"),
			VerityConfig:   cmd.String("verity-config"),
			TargetRegistry: cmd.String("target-registry"),
			ChartRegistry:  cmd.String("chart-registry"),
			ExcludeNames:   parseNameSet(cmd.String("exclude-names")),
			DryRun:         cmd.Bool("dry-run"),
			Strict:         cmd.Bool("strict"),
		}

		result, err := chartgen.Run(cfg)
		if err != nil {
			return fmt.Errorf("chart-gen failed: %w", err)
		}

		if cfg.DryRun {
			out, err := json.MarshalIndent(result, "", "  ")
			if err != nil {
				return fmt.Errorf("failed to marshal dry-run result: %w", err)
			}
			fmt.Fprintln(os.Stdout, string(out))
		}

		return nil
	},
}
