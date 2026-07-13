package cmd

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"

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
			Usage: "target registry for patched images (e.g. verity.supply)",
		},
		&cli.StringFlag{
			Name:  "reports-dir",
			Usage: "directory containing pre-patch Trivy vulnerability reports",
		},
		&cli.StringFlag{
			Name:  "post-reports-dir",
			Usage: "directory containing post-patch Trivy vulnerability reports (for before/after comparison)",
		},
		&cli.StringFlag{
			Name:  "integer-catalog",
			Usage: "path to integer catalog.json (adds Wolfi-based rebuilds section)",
		},
	},
	Action: runCatalog,
}

func runCatalog(_ context.Context, cmd *cli.Command) error {
	output := cmd.String("output")
	imagesJSON := cmd.String("images-json")
	registry := cmd.String("registry")
	reportsDir := cmd.String("reports-dir")
	postReportsDir := cmd.String("post-reports-dir")
	integerCatalog := cmd.String("integer-catalog")

	siteData, err := internal.GenerateSiteData(imagesJSON, reportsDir, postReportsDir, registry)
	if err != nil {
		return fmt.Errorf("failed to generate site data: %w", err)
	}

	if err := internal.MergeIntegerCatalog(siteData, integerCatalog); err != nil {
		return fmt.Errorf("failed to merge integer catalog: %w", err)
	}

	if err := internal.WriteSiteData(siteData, output); err != nil {
		return fmt.Errorf("failed to write catalog: %w", err)
	}

	fmt.Printf("Site catalog → %s (%d patched, %d integer)\n",
		output, len(siteData.Images), len(siteData.IntegerImages))
	return nil
}
