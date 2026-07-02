package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/urfave/cli/v3"

	"github.com/verity-org/verity/internal/integer/apkindex"
	intconfig "github.com/verity-org/verity/internal/integer/config"
	"github.com/verity-org/verity/internal/integer/discovery"
)

var integerDiscoverCmd = &cli.Command{
	Name:  "discover",
	Usage: "List all image+variant combinations as a JSON array for CI",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "config",
			Aliases: []string{"c"},
			Usage:   "Path to integer.yaml",
			Value:   "integer.yaml",
		},
		&cli.StringFlag{
			Name:  "images-dir",
			Usage: "Path to the images/ directory",
			Value: "images",
		},
		&cli.StringFlag{
			Name:  "apkindex-url",
			Usage: "Wolfi APKINDEX URL (empty disables version discovery)",
			Value: apkindex.DefaultAPKINDEXURL,
		},
		&cli.StringFlag{
			Name:  "cache-dir",
			Usage: "Directory for caching APKINDEX data (empty disables cache)",
			Value: os.TempDir(),
		},
		&cli.StringFlag{
			Name:  "gen-dir",
			Usage: "Directory for generated apko configs (default: system temp)",
		},
		&cli.StringFlag{
			Name:  "only",
			Usage: "Comma-separated list of image names to include (empty = all)",
		},
		&cli.BoolFlag{
			Name:  "preflight",
			Usage: "Enable preflight digest-based skip logic (compares upstream digests with manifest)",
		},
		&cli.StringFlag{
			Name:  "github-repo",
			Usage: "GitHub repository (owner/repo) for preflight manifest lookup",
		},
		&cli.StringFlag{
			Name:  "reports-branch",
			Usage: "Branch where preflight-manifest.json is stored",
			Value: "reports",
		},
	},
	Action: func(_ context.Context, cmd *cli.Command) error {
		cfg, err := intconfig.LoadConfig(cmd.String("config"))
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}

		var pkgs []apkindex.Package
		if url := cmd.String("apkindex-url"); url != "" {
			pkgs, err = apkindex.Fetch(url, cmd.String("cache-dir"), apkindex.DefaultCacheMaxAge)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: APKINDEX unavailable (%v) — using versions map only\n", err)
				pkgs = nil
			}
		}

		imagesDir, err := filepath.Abs(cmd.String("images-dir"))
		if err != nil {
			return fmt.Errorf("resolving images dir: %w", err)
		}

		imgs, err := discovery.DiscoverFromFiles(discovery.Options{
			ImagesDir: imagesDir,
			Registry:  cfg.Target.Registry,
			Packages:  pkgs,
			GenDir:    cmd.String("gen-dir"),
		})
		if err != nil {
			return fmt.Errorf("discovering images: %w", err)
		}

		// --only: filter to specific image names
		if only := cmd.String("only"); only != "" {
			imgs = filterIntegerImagesByName(imgs, only)
			fmt.Fprintf(os.Stderr, "Filtered to %d images matching --only=%s\n", len(imgs), only)
		}

		// --preflight: for Integer images, preflight is a no-op for now since
		// Integer builds from source (no upstream digest to compare). The flag
		// is accepted for workflow symmetry but currently just logs.
		if cmd.Bool("preflight") {
			fmt.Fprintf(os.Stderr, "Preflight: Integer images build from source — no digest filtering applied\n")
		}

		// CI consumers (jq 'length') require a JSON array even for zero matches.
		if imgs == nil {
			imgs = []discovery.DiscoveredImage{}
		}

		out, err := json.MarshalIndent(imgs, "", "  ")
		if err != nil {
			return fmt.Errorf("marshalling output: %w", err)
		}
		fmt.Fprintln(os.Stdout, string(out))
		return nil
	},
}

// filterIntegerImagesByName filters images to only those whose Name matches one
// of the comma-separated names.
func filterIntegerImagesByName(images []discovery.DiscoveredImage, names string) []discovery.DiscoveredImage {
	allowed := parseNameSet(names)
	if allowed == nil {
		return images
	}

	var filtered []discovery.DiscoveredImage
	for _, img := range images {
		if _, ok := allowed[img.Name]; ok {
			filtered = append(filtered, img)
		}
	}
	return filtered
}
