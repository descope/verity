package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/urfave/cli/v2"

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

func runIntegerCatalog(c *cli.Context) error {
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

	var eolClient *eol.Client
	if c.Bool("fetch-eol") {
		eolClient = eol.NewClient()
	}

	cat, err := catalog.Generate(
		c.String("images-dir"),
		c.String("reports-dir"),
		cfg.Target.Registry,
		pkgs,
		eolClient,
	)
	if err != nil {
		return fmt.Errorf("generating catalog: %w", err)
	}

	out, err := json.MarshalIndent(cat, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling catalog: %w", err)
	}

	output := c.String("output")
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
