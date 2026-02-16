package cmd

import (
	"fmt"

	"github.com/urfave/cli/v2"

	"github.com/verity-org/verity/internal"
)

// ListCommand lists all images from values.yaml without patching.
var ListCommand = &cli.Command{
	Name:  "list",
	Usage: "List images from values.yaml (dry run)",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "images",
			Aliases: []string{"i"},
			Value:   "values.yaml",
			Usage:   "path to images values.yaml",
		},
	},
	Action: runList,
}

func runList(c *cli.Context) error {
	imagesFile := c.String("images")

	overrides, err := parseOverridesFromFile(imagesFile)
	if err != nil {
		return err
	}

	images, err := internal.ParseImagesFile(imagesFile)
	if err != nil {
		return fmt.Errorf("failed to parse %s: %w", imagesFile, err)
	}

	images = internal.ApplyOverrides(images, overrides)

	fmt.Printf("Images from %s:\n", imagesFile)
	for _, img := range images {
		fmt.Printf("  %s  (%s)\n", img.Reference(), img.Path)
	}
	fmt.Printf("\nTotal: %d images\n", len(images))
	return nil
}
