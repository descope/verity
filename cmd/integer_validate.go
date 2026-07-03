package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/urfave/cli/v3"

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
			Name:  "bespoke-dir",
			Usage: "Path to the packages/bespoke/ directory; bespoke melange yamls referenced from images/ are cross-checked here",
			Value: filepath.Join("packages", "bespoke"),
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

func runIntegerValidate(_ context.Context, cmd *cli.Command) error {
	cfgPath := cmd.String("config")
	imagesDir := cmd.String("images-dir")
	bespokeDir := cmd.String("bespoke-dir")

	// Stat bespokeDir once. When the directory is missing, validateBespokeRefs
	// emits a single summary FAIL per affected def instead of N per-type
	// ENOENTs (which previously also double-counted with reportOrphanBespoke's
	// own summary). When the directory exists, the per-file behavior is
	// unchanged.
	bespokeDirExists := isExistingDir(bespokeDir)

	var pkgs []apkindex.Package
	if url := cmd.String("apkindex-url"); url != "" {
		var err error
		pkgs, err = apkindex.Fetch(url, cmd.String("cache-dir"), apkindex.DefaultCacheMaxAge)
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

	imageFiles, err := intconfig.ImageFilePaths(imagesDir)
	if err != nil {
		return fmt.Errorf("reading images directory: %w", err)
	}

	// Track which bespoke files are referenced so we can flag orphans below.
	referencedBespoke := map[string]string{} // bespoke filename → image yaml path

	checked := 0
	for _, defPath := range imageFiles {
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

		apkFails, skip := validateAgainstAPKINDEX(def, defPath, pkgs)
		failures += apkFails
		if skip {
			continue
		}

		// Bespoke cross-checks: every type that declares melange.bespoke must
		// reference a packages/bespoke/<file>.yaml whose package.name appears
		// in the type's apko packages: list. Without this guard, a typo (or
		// a forgotten image-yaml entry) silently produces an apk that apko
		// can't resolve at publish time — exactly the failure mode of #297.
		bespokeFails := validateBespokeRefs(def, defPath, bespokeDir, referencedBespoke, bespokeDirExists)
		if bespokeFails > 0 {
			failures += bespokeFails
			continue
		}

		fmt.Fprintf(os.Stdout, "OK   %s (%d types, %d declared versions)\n",
			defPath, len(def.Types), len(def.Versions))
		checked++
	}

	// Detect orphan bespoke yamls: files in packages/bespoke/ that no image
	// references. These are dead weight and usually indicate someone added a
	// bespoke package but forgot to wire it into images/.
	if orphanFailures := reportOrphanBespoke(bespokeDir, referencedBespoke, bespokeDirExists); orphanFailures > 0 {
		failures += orphanFailures
	}

	if failures > 0 {
		return fmt.Errorf("%d error(s): %w", failures, errIntegerValidationFailed)
	}

	fmt.Fprintf(os.Stdout, "\nAll configs valid (%d images checked)\n", checked)
	return nil
}
