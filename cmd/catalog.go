package cmd

import (
	"fmt"

	"github.com/urfave/cli/v2"

	"github.com/verity-org/verity/internal"
)

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
			Name:    "images",
			Aliases: []string{"i"},
			Value:   "values.yaml",
			Usage:   "path to images values.yaml",
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
	imagesFile := c.String("images")
	registry := c.String("registry")
	reportsDir := c.String("reports-dir")

	if err := internal.GenerateSiteData(imagesFile, reportsDir, registry, output); err != nil {
		return fmt.Errorf("failed to generate site data: %w", err)
	}
	fmt.Printf("Site catalog → %s\n", output)
	return nil
}
