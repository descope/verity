package cmd

import (
	"errors"
	"fmt"

	"github.com/urfave/cli/v2"

	"github.com/verity-org/verity/internal"
)

var ErrMissingFlag = errors.New("required flag missing")

// CatalogCommand generates the site catalog JSON from patch reports.
var CatalogCommand = &cli.Command{
	Name:  "catalog",
	Usage: "Generate site catalog JSON from patch reports",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:     "output",
			Aliases:  []string{"o"},
			Required: true,
			Usage:    "output path for catalog JSON (e.g. site/src/data/catalog.json)",
		},
		&cli.StringFlag{
			Name:     "images-json",
			Aliases:  []string{"j"},
			Required: true,
			Usage:    "path to images.json from sign-and-attest script",
		},
		&cli.StringFlag{
			Name:  "registry",
			Usage: "target registry for patched images (e.g. ghcr.io/verity-org)",
		},
		&cli.StringFlag{
			Name:     "reports-dir",
			Required: true,
			Usage:    "directory containing Trivy vulnerability reports",
		},
	},
	Action: runCatalog,
}

func runCatalog(c *cli.Context) error {
	output := c.String("output")
	imagesJSON := c.String("images-json")
	registry := c.String("registry")
	reportsDir := c.String("reports-dir")

	if imagesJSON == "" {
		return fmt.Errorf("%w: --images-json is required", ErrMissingFlag)
	}

	if err := internal.GenerateSiteDataFromJSON(imagesJSON, reportsDir, registry, output); err != nil {
		return fmt.Errorf("failed to generate site data from JSON: %w", err)
	}
	fmt.Printf("Site catalog → %s\n", output)
	return nil
}
