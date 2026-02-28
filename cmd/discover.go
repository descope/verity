package cmd

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"

	"github.com/urfave/cli/v2"

	"github.com/verity-org/verity/internal/discovery"
)

// DiscoverCommand enumerates all image+tag combos from three sources:
//   - copa-config.yaml  — standalone images (Copa's domain)
//   - Chart.yaml        — Helm chart dependencies (standard Helm format)
//   - verity.yaml       — tag variant overrides (verity-specific)
var DiscoverCommand = &cli.Command{
	Name:  "discover",
	Usage: "Enumerate all image+tag combos from copa-config.yaml, Chart.yaml, and verity.yaml overrides",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:     "config",
			Aliases:  []string{"c"},
			Usage:    "Path to copa-config.yaml (standalone images)",
			Required: true,
		},
		&cli.StringFlag{
			Name:  "target-registry",
			Usage: "Override the target registry from config (e.g., ghcr.io/verity-org)",
		},
		&cli.StringFlag{
			Name:  "charts-file",
			Usage: "Helm Chart.yaml whose dependencies: provides chart images",
			Value: "Chart.yaml",
		},
		&cli.StringFlag{
			Name:  "verity-config",
			Usage: "Path to verity.yaml (tag variant overrides)",
			Value: "verity.yaml",
		},
	},
	Action: func(c *cli.Context) error {
		cfg, err := discovery.LoadConfig(c.String("config"))
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		charts, err := discovery.LoadChartsFile(c.String("charts-file"))
		if err != nil {
			return fmt.Errorf("failed to load charts file: %w", err)
		}
		cfg.Charts = append(cfg.Charts, charts...)

		vc, err := discovery.LoadVerityConfig(c.String("verity-config"))
		if err != nil {
			return fmt.Errorf("failed to load verity config: %w", err)
		}

		// Merge overrides: verity.yaml takes precedence over copa-config.yaml.
		overrides := maps.Clone(cfg.Overrides)
		if overrides == nil {
			overrides = maps.Clone(vc.Overrides)
		} else {
			maps.Copy(overrides, vc.Overrides)
		}
		images, err := discovery.Discover(cfg, c.String("target-registry"), overrides)
		if err != nil {
			return fmt.Errorf("failed to discover images: %w", err)
		}

		out, err := json.MarshalIndent(images, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal output: %w", err)
		}

		fmt.Fprintln(os.Stdout, string(out))
		return nil
	},
}
