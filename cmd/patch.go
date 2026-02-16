package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/urfave/cli/v2"

	"github.com/verity-org/verity/internal"
)

// PatchCommand patches a single image and writes the result.
var PatchCommand = &cli.Command{
	Name:  "patch",
	Usage: "Patch a single container image",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:     "image",
			Required: true,
			Usage:    "image reference to patch (e.g. quay.io/prometheus/prometheus:v3.9.1)",
		},
		&cli.StringFlag{
			Name:  "registry",
			Usage: "target registry for patched images (e.g. ghcr.io/verity-org)",
		},
		&cli.StringFlag{
			Name:  "buildkit-addr",
			Usage: "BuildKit address for Copa (e.g. docker-container://buildkitd)",
		},
		&cli.StringFlag{
			Name:  "report-dir",
			Usage: "directory to store Trivy JSON reports (default: temp dir)",
		},
		&cli.StringFlag{
			Name:     "result-dir",
			Required: true,
			Usage:    "directory to write patch result JSON",
		},
	},
	Action: runPatch,
}

func runPatch(c *cli.Context) error {
	imageRef := c.String("image")
	registry := c.String("registry")
	buildkitAddr := c.String("buildkit-addr")
	reportDir := c.String("report-dir")
	resultDir := c.String("result-dir")

	tmpDir, err := os.MkdirTemp("", "verity-patch-")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to clean up temp dir: %v\n", err)
		}
	}()

	rDir := reportDir
	if rDir == "" {
		rDir = filepath.Join(tmpDir, "reports")
	}

	opts := internal.PatchOptions{
		TargetRegistry: registry,
		BuildKitAddr:   buildkitAddr,
		ReportDir:      rDir,
		WorkDir:        tmpDir,
	}

	fmt.Printf("Patching %s ...\n", imageRef)
	ctx := context.Background()
	if err := internal.PatchSingleImage(ctx, imageRef, opts, resultDir); err != nil {
		return fmt.Errorf("patch failed: %w", err)
	}
	fmt.Println("Done.")
	return nil
}
