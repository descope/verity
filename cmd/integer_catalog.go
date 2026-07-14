package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"

	"github.com/verity-org/verity/internal/integer/apkindex"
	"github.com/verity-org/verity/internal/integer/catalog"
	intconfig "github.com/verity-org/verity/internal/integer/config"
	"github.com/verity-org/verity/internal/integer/eol"
)

var integerCatalogCmd = &cli.Command{
	Name:  "catalog",
	Usage: "Generate catalog.json for the verity website",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "images-dir",
			Aliases: []string{"i"},
			Usage:   "Path to the images/ directory",
			Value:   "images",
		},
		&cli.StringFlag{
			Name:  "reports-dir",
			Usage: "Path to checked-out reports directory (reports branch)",
		},
		&cli.StringFlag{
			Name:  "expected-batch-id",
			Usage: "Only accept reports produced by this nightly batch",
		},
		&cli.StringFlag{
			Name:    "config",
			Aliases: []string{"c"},
			Usage:   "Path to integer.yaml",
			Value:   "integer.yaml",
		},
		&cli.StringFlag{
			Name:    "output",
			Aliases: []string{"o"},
			Usage:   "Output file path (- for stdout)",
			Value:   "catalog.json",
		},
		&cli.StringFlag{
			Name:  "apkindex-url",
			Usage: "Wolfi APKINDEX URL",
			Value: apkindex.DefaultAPKINDEXURL,
		},
		&cli.StringFlag{
			Name:  "cache-dir",
			Usage: "Directory for caching APKINDEX data",
			Value: os.TempDir(),
		},
		&cli.BoolFlag{
			Name:  "fetch-eol",
			Usage: "Fetch EOL data from endoflife.date API",
			Value: true,
		},
	},
	Action: runIntegerCatalog,
}

func runIntegerCatalog(_ context.Context, cmd *cli.Command) error {
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

	var eolFetcher eol.Fetcher
	if cmd.Bool("fetch-eol") {
		eolFetcher = eol.NewClient()
	}

	cat, err := catalog.GenerateWithOptions(catalog.Options{
		ImagesDir:       cmd.String("images-dir"),
		ReportsDir:      cmd.String("reports-dir"),
		Registry:        cfg.Target.Registry,
		Packages:        pkgs,
		EOLFetcher:      eolFetcher,
		ExpectedBatchID: cmd.String("expected-batch-id"),
	})
	if err != nil {
		return fmt.Errorf("generating catalog: %w", err)
	}

	out, err := json.MarshalIndent(cat, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling catalog: %w", err)
	}

	output := cmd.String("output")
	if output == "-" {
		fmt.Println(string(out))
		return nil
	}

	if err := os.WriteFile(output, append(out, '\n'), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", output, err)
	}

	fmt.Fprintf(os.Stdout, "Catalog → %s (%d images)\n", output, len(cat.Images))
	return nil
}
