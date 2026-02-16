package cmd

import (
	"errors"
	"fmt"

	"github.com/urfave/cli/v2"

	"github.com/verity-org/verity/internal"
)

var errRegistryRequired = errors.New("--registry is required when --publish is set")

// AssembleCommand creates wrapper charts from patch results.
var AssembleCommand = &cli.Command{
	Name:  "assemble",
	Usage: "Create and publish wrapper charts from patch results",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:     "manifest",
			Required: true,
			Usage:    "path to discovery manifest.json",
		},
		&cli.StringFlag{
			Name:     "results-dir",
			Required: true,
			Usage:    "directory with patch result JSONs",
		},
		&cli.StringFlag{
			Name:  "reports-dir",
			Usage: "directory with Trivy reports",
		},
		&cli.StringFlag{
			Name:  "output-dir",
			Value: ".verity/charts",
			Usage: "where to generate charts locally",
		},
		&cli.StringFlag{
			Name:  "registry",
			Usage: "OCI registry for version querying and publishing",
		},
		&cli.BoolFlag{
			Name:  "publish",
			Usage: "actually push to OCI (without it, just creates locally)",
		},
	},
	Action: runAssemble,
}

func runAssemble(c *cli.Context) error {
	manifestPath := c.String("manifest")
	resultsDir := c.String("results-dir")
	reportsDir := c.String("reports-dir")
	outputDir := c.String("output-dir")
	registry := c.String("registry")
	publish := c.Bool("publish")

	if publish && registry == "" {
		return errRegistryRequired
	}

	fmt.Printf("Assembling wrapper charts from %s\n", manifestPath)
	if publish {
		fmt.Printf("Publishing to %s\n", registry)
	}

	err := internal.AssembleResults(manifestPath, resultsDir, reportsDir, outputDir, registry, publish)
	if err != nil {
		return fmt.Errorf("assemble failed: %w", err)
	}

	fmt.Println("\nAssemble complete")
	return nil
}
