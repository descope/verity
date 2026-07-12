package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"

	intconfig "github.com/verity-org/verity/internal/integer/config"
)

var integerMetadataCmd = &cli.Command{
	Name:  "metadata",
	Usage: "Write image definition metadata to GitHub Actions output",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:     "image",
			Aliases:  []string{"i"},
			Usage:    "Declared image name",
			Required: true,
		},
		&cli.StringFlag{
			Name:  "images-dir",
			Usage: "Path to the images/ directory",
			Value: "images",
		},
		&cli.StringFlag{
			Name:     "github-output",
			Usage:    "Append title and description to this GitHub output file",
			Required: true,
		},
	},
	Action: runIntegerMetadata,
}

func runIntegerMetadata(_ context.Context, cmd *cli.Command) (retErr error) {
	image := cmd.String("image")
	def, err := intconfig.LoadImageByName(cmd.String("images-dir"), image)
	if err != nil {
		return fmt.Errorf("loading image %q: %w", image, err)
	}

	outputPath := cmd.String("github-output")
	output, err := os.OpenFile(outputPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("opening GitHub output %q: %w", outputPath, err)
	}
	defer func() {
		if closeErr := output.Close(); closeErr != nil {
			retErr = errors.Join(retErr, fmt.Errorf("closing GitHub output %q: %w", outputPath, closeErr))
		}
	}()

	if _, err := fmt.Fprintf(
		output,
		"title<<__VERITY_IMAGE_TITLE__\n%s\n__VERITY_IMAGE_TITLE__\ndescription<<__VERITY_IMAGE_DESCRIPTION__\n%s\n__VERITY_IMAGE_DESCRIPTION__\n",
		def.Name, def.Description,
	); err != nil {
		return fmt.Errorf("writing GitHub output %q: %w", outputPath, err)
	}
	return nil
}
