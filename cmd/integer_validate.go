package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/urfave/cli/v2"

	"github.com/verity-org/verity/internal/integer/apkindex"
	intconfig "github.com/verity-org/verity/internal/integer/config"
)

var errIntegerValidationFailed = errors.New("validation failed")

var integerValidateCmd = &cli.Command{
	Name:  "validate",
	Usage: "Schema-validate all image configs in images/",
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
			Usage: "Wolfi APKINDEX URL; when set, verifies upstream packages exist (skipped if empty)",
			Value: "",
		},
		&cli.StringFlag{
			Name:  "cache-dir",
			Usage: "Directory for caching APKINDEX data",
			Value: os.TempDir(),
		},
	},
	Action: runIntegerValidate,
}

func runIntegerValidate(c *cli.Context) error {
	cfgPath := c.String("config")
	imagesDir := c.String("images-dir")

	var pkgs []apkindex.Package
	if url := c.String("apkindex-url"); url != "" {
		var err error
		pkgs, err = apkindex.Fetch(url, c.String("cache-dir"), apkindex.DefaultCacheMaxAge)
		if err != nil {
			return fmt.Errorf("fetching APKINDEX: %w", err)
		}
	}

	failures := 0

	if _, err := intconfig.LoadConfig(cfgPath); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL %s: %v\n", cfgPath, err)
		failures++
	} else {
		fmt.Fprintf(os.Stdout, "OK   %s\n", cfgPath)
	}

	entries, err := os.ReadDir(imagesDir)
	if err != nil {
		return fmt.Errorf("reading images directory: %w", err)
	}

	checked := 0
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}

		defPath := filepath.Join(imagesDir, entry.Name())
		def, err := intconfig.LoadImage(defPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FAIL %s: %v\n", defPath, err)
			failures++
			continue
		}
		if err := intconfig.Validate(def); err != nil {
			fmt.Fprintf(os.Stderr, "FAIL %s: %v\n", defPath, err)
			failures++
			continue
		}

		if len(pkgs) > 0 {
			if found := apkindex.DiscoverVersions(pkgs, def.Upstream.Package); len(found) == 0 {
				fmt.Fprintf(os.Stderr, "FAIL %s: upstream package %q not found in APKINDEX\n",
					defPath, def.Upstream.Package)
				failures++
				continue
			}
		}

		fmt.Fprintf(os.Stdout, "OK   %s (%d types, %d declared versions)\n",
			defPath, len(def.Types), len(def.Versions))
		checked++
	}

	if failures > 0 {
		return fmt.Errorf("%d error(s): %w", failures, errIntegerValidationFailed)
	}

	fmt.Fprintf(os.Stdout, "\nAll configs valid (%d images checked)\n", checked)
	return nil
}
