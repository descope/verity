package cmd

import (
	"fmt"
	"log"

	"github.com/urfave/cli/v2"

	"github.com/verity-org/verity/internal"
)

// DiscoverCommand scans images from values.yaml and outputs a GitHub Actions matrix
var DiscoverCommand = &cli.Command{
	Name:  "discover",
	Usage: "Scan images and output a GitHub Actions matrix",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "images",
			Aliases: []string{"i"},
			Value:   "values.yaml",
			Usage:   "path to images values.yaml",
		},
		&cli.StringFlag{
			Name:  "chart-file",
			Usage: "path to Chart.yaml (enables chart-based discovery)",
		},
		&cli.StringFlag{
			Name:    "discover-dir",
			Aliases: []string{"d"},
			Value:   ".verity",
			Usage:   "output directory for discover artifacts",
		},
	},
	Action: runDiscover,
}

func runDiscover(c *cli.Context) error {
	imagesFile := c.String("images")
	chartFile := c.String("chart-file")
	discoverDir := c.String("discover-dir")

	overrides := parseOverridesFromFile(imagesFile)

	var manifest *internal.DiscoveryManifest
	var err error

	if chartFile != "" {
		// Chart-based discovery: scan Chart.yaml dependencies + merge into images file.
		manifest, err = internal.DiscoverImages(chartFile, imagesFile, discoverDir)
		if err != nil {
			return fmt.Errorf("discovery failed: %w", err)
		}
	} else {
		// Flat discovery: just parse the images file.
		images, err := internal.ParseImagesFile(imagesFile)
		if err != nil {
			return fmt.Errorf("discovery failed: %w", err)
		}

		// Convert to discovery format
		manifest = &internal.DiscoveryManifest{
			Images: make([]internal.ImageDiscovery, len(images)),
		}
		for i, img := range images {
			manifest.Images[i] = internal.ImageDiscovery(img)
		}
	}

	// Apply image tag overrides (e.g. distroless → debian) so the matrix
	// contains Copa-compatible refs. Apply to both flat Images and Charts[*].Images.
	if len(overrides) > 0 {
		internal.ApplyOverridesToManifest(manifest, overrides)
	}

	matrix := internal.GenerateMatrix(manifest)

	if err := internal.WriteDiscoveryOutput(manifest, matrix, discoverDir); err != nil {
		return fmt.Errorf("failed to write discovery output: %w", err)
	}

	fmt.Printf("\nDiscovery complete: %d unique images\n", len(matrix.Include))
	fmt.Printf("  Manifest → %s/manifest.json\n", discoverDir)
	fmt.Printf("  Matrix   → %s/matrix.json\n", discoverDir)
	return nil
}

// parseOverridesFromFile loads image tag overrides from the images file, if present.
func parseOverridesFromFile(imagesFile string) []internal.ImageOverride {
	if imagesFile == "" {
		return nil
	}
	overrides, err := internal.ParseOverrides(imagesFile)
	if err != nil {
		log.Fatalf("Failed to parse overrides from %s: %v", imagesFile, err)
	}
	if len(overrides) > 0 {
		fmt.Printf("Loaded %d image override(s)\n", len(overrides))
		for _, o := range overrides {
			fmt.Printf("  %s: %q → %q\n", o.Repository, o.From, o.To)
		}
	}
	return overrides
}
