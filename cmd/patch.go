package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"

	"github.com/verity-org/verity/internal/patch"
)

// PatchCommand wraps copa's pkg/patch.Patch entry point so verity can patch a
// single image without shelling out to a separate `copa` binary. The flag
// surface mirrors `.github/scripts/patch-image.sh`'s prior `copa patch`
// invocation so swapping one for the other is drop-in.
var PatchCommand = &cli.Command{
	Name:  "patch",
	Usage: "Patch a single image via Copa (imported as a library)",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:     "image",
			Aliases:  []string{"i"},
			Usage:    "Source image reference (e.g. mirror.gcr.io/library/nginx:1.29.3)",
			Required: true,
		},
		&cli.StringFlag{
			Name:     "tag",
			Aliases:  []string{"t"},
			Usage:    "Output tag for the patched image (e.g. 1.29.3-linux-amd64-patched)",
			Required: true,
		},
		&cli.StringFlag{
			Name:     "report",
			Aliases:  []string{"r"},
			Usage:    "Path to Trivy JSON vulnerability report for --image",
			Required: true,
		},
		&cli.StringFlag{
			Name:  "pkg-types",
			Usage: "Comma-separated package ecosystems to patch (os|library|os,library)",
			Value: "os,library",
		},
		&cli.StringFlag{
			Name:  "library-patch-level",
			Usage: "How aggressively to bump library versions (patch|minor|major)",
			Value: "patch",
		},
		&cli.StringFlag{
			Name:  "toolchain-patch-level",
			Usage: "How aggressively to upgrade the Go toolchain (patch|minor|major)",
			Value: "patch",
		},
		&cli.BoolFlag{
			Name:  "push",
			Usage: "Push the patched image to the registry after building",
		},
		&cli.StringFlag{
			Name:  "buildkit-addr",
			Usage: "BuildKit endpoint (e.g. buildx://copa-builder or tcp://127.0.0.1:1234)",
		},
		&cli.DurationFlag{
			Name:  "timeout",
			Usage: "Upper bound on the whole patch operation (e.g. 30m)",
			Value: patch.DefaultTimeout,
		},
		&cli.StringFlag{
			Name:  "platform",
			Usage: "Single platform to build for (e.g. linux/amd64). Empty = copa default",
		},
		&cli.StringFlag{
			Name: "go-vcs-url",
			Usage: "Go module VCS URL used when patching stripped/distroless " +
				"Go binaries (overrides copa's embedded buildinfo auto-detect)",
		},
	},
	Action: patchAction,
}

func patchAction(ctx context.Context, cmd *cli.Command) error {
	cfg := &patch.Config{
		Image:               cmd.String("image"),
		PatchedTag:          cmd.String("tag"),
		Report:              cmd.String("report"),
		PkgTypes:            cmd.String("pkg-types"),
		LibraryPatchLevel:   cmd.String("library-patch-level"),
		ToolchainPatchLevel: cmd.String("toolchain-patch-level"),
		Push:                cmd.Bool("push"),
		BuildKitAddr:        cmd.String("buildkit-addr"),
		Timeout:             cmd.Duration("timeout"),
		Platform:            cmd.String("platform"),
		GoVCSURL:            cmd.String("go-vcs-url"),
	}

	if err := patch.Run(ctx, cfg); err != nil {
		// ErrNoUpdatesFound is informational, not an error: the image is
		// already clean. Print the legacy "no package updates found ..."
		// string to stderr so CI logs stay grep-compatible with the
		// pre-migration format; patch-image.sh's retry-branch grep only
		// runs on non-zero exits, and we exit 0 here, so this is for log
		// readability and defensive back-compat — not control flow.
		if errors.Is(err, patch.ErrNoUpdatesFound) {
			fmt.Fprintf(os.Stderr, "no package updates found for image %s\n", cfg.Image)
			return nil
		}
		return fmt.Errorf("verity patch: %w", err)
	}
	return nil
}
