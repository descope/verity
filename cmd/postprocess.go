package cmd

import (
	"fmt"

	"github.com/urfave/cli/v2"

	"github.com/verity-org/verity/internal"
)

// PostProcessCommand processes Copa's output and generates inputs for downstream jobs.
var PostProcessCommand = &cli.Command{
	Name:  "post-process",
	Usage: "Process Copa bulk patch output and generate matrices/manifests",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:     "copa-output",
			Required: true,
			Usage:    "path to Copa's --output-json file",
		},
		&cli.StringFlag{
			Name:     "chart-map",
			Required: true,
			Usage:    "path to chart-image-map.yaml",
		},
		&cli.StringFlag{
			Name:     "registry",
			Required: true,
			Usage:    "target registry prefix (e.g., ghcr.io/verity-org)",
		},
		&cli.StringFlag{
			Name:  "output-dir",
			Value: ".verity",
			Usage: "output directory for generated files",
		},
		&cli.BoolFlag{
			Name:  "skip-digest-lookup",
			Usage: "skip registry digest lookup (for testing)",
		},
	},
	Action: runPostProcess,
}

func runPostProcess(c *cli.Context) error {
	opts := internal.PostProcessOptions{
		CopaOutputPath:   c.String("copa-output"),
		ChartMapPath:     c.String("chart-map"),
		RegistryPrefix:   c.String("registry"),
		OutputDir:        c.String("output-dir"),
		SkipDigestLookup: c.Bool("skip-digest-lookup"),
	}

	fmt.Println("Post-processing Copa results...")
	fmt.Printf("  Copa output: %s\n", opts.CopaOutputPath)
	fmt.Printf("  Chart map: %s\n", opts.ChartMapPath)
	fmt.Printf("  Registry: %s\n", opts.RegistryPrefix)
	fmt.Printf("  Output dir: %s\n", opts.OutputDir)

	result, err := internal.PostProcessCopaResults(opts)
	if err != nil {
		return fmt.Errorf("post-process failed: %w", err)
	}

	fmt.Println("\nResults:")
	fmt.Printf("  Patched: %d\n", result.PatchedCount)
	fmt.Printf("  Skipped: %d\n", result.SkippedCount)
	fmt.Printf("  Failed: %d\n", result.FailedCount)
	fmt.Printf("  Charts: %d\n", result.ChartCount)
	fmt.Println("\nGenerated files:")
	fmt.Printf("  Matrix: %s\n", result.MatrixPath)
	fmt.Printf("  Manifest: %s\n", result.ManifestPath)
	fmt.Printf("  Results dir: %s\n", result.ResultsDir)

	if !result.HasImages {
		fmt.Println("\nNo images to attest (all skipped or failed)")
	}
	if !result.HasCharts {
		fmt.Println("\nNo charts to assemble")
	}

	return nil
}
