package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/urfave/cli/v2"

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
	},
	Action: func(c *cli.Context) error {
		cfg, err := intconfig.LoadConfig(c.String("config"))
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}

		var pkgs []apkindex.Package
		if url := c.String("apkindex-url"); url != "" {
			pkgs, err = apkindex.Fetch(url, c.String("cache-dir"), apkindex.DefaultCacheMaxAge)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: APKINDEX unavailable (%v) — using versions map only\n", err)
				pkgs = nil
			}
		}

		imagesDir, err := filepath.Abs(c.String("images-dir"))
		if err != nil {
			return fmt.Errorf("resolving images dir: %w", err)
		}

		imgs, err := discovery.DiscoverFromFiles(discovery.Options{
			ImagesDir: imagesDir,
			Registry:  cfg.Target.Registry,
			Packages:  pkgs,
			GenDir:    c.String("gen-dir"),
		})
		if err != nil {
			return fmt.Errorf("discovering images: %w", err)
		}

		out, err := json.MarshalIndent(imgs, "", "  ")
		if err != nil {
			return fmt.Errorf("marshalling output: %w", err)
		}
		fmt.Fprintln(os.Stdout, string(out))
		return nil
	},
}
