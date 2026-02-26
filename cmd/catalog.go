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
			Name:  "reports-dir",
			Usage: "directory containing pre-patch Trivy vulnerability reports",
		},
		&cli.StringFlag{
			Name:  "post-reports-dir",
			Usage: "directory containing post-patch Trivy vulnerability reports (for before/after comparison)",
		},
	},
	Action: runCatalog,
}

func runCatalog(c *cli.Context) error {
	output := c.String("output")
	imagesJSON := c.String("images-json")
	registry := c.String("registry")
	reportsDir := c.String("reports-dir")
	postReportsDir := c.String("post-reports-dir")

	if err := internal.GenerateSiteDataFromJSON(imagesJSON, reportsDir, postReportsDir, registry, output); err != nil {
		return fmt.Errorf("failed to generate site data from JSON: %w", err)
	}
	fmt.Printf("Site catalog → %s\n", output)
	return nil
}
