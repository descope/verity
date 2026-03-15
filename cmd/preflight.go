package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"

	"github.com/verity-org/verity/internal/preflight"
)

var errMissingToken = errors.New("GH_TOKEN or GITHUB_TOKEN must be set")

// PreflightCommand is the top-level "preflight" subcommand group for managing
// the preflight manifest used to skip unnecessary builds.
var PreflightCommand = &cli.Command{
	Name:  "preflight",
	Usage: "Manage preflight manifest for build skipping",
	Commands: []*cli.Command{
		{
			Name:  "update-manifest",
			Usage: "Update the preflight manifest with a new image entry",
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:     "github-repo",
					Usage:    "GitHub repository (owner/repo)",
					Required: true,
				},
				&cli.StringFlag{
					Name:  "reports-branch",
					Usage: "Branch where preflight-manifest.json is stored",
					Value: "reports",
				},
				&cli.StringFlag{
					Name:     "image",
					Usage:    "Image name",
					Required: true,
				},
				&cli.StringFlag{
					Name:     "tag",
					Usage:    "Image tag",
					Required: true,
				},
				&cli.StringFlag{
					Name:  "upstream-digest",
					Usage: "Upstream image digest (sha256:...)",
				},
				&cli.IntFlag{
					Name:  "patched-vulns",
					Usage: "Number of fixable vulnerabilities remaining after patching",
					Value: 0,
				},
			},
			Action: func(_ context.Context, cmd *cli.Command) error {
				token := os.Getenv("GH_TOKEN")
				if token == "" {
					token = os.Getenv("GITHUB_TOKEN")
				}
				if token == "" {
					return errMissingToken
				}

				entry := preflight.UpdateEntry{
					ImageName:      cmd.String("image"),
					Tag:            cmd.String("tag"),
					UpstreamDigest: cmd.String("upstream-digest"),
					PatchedVulns:   cmd.Int("patched-vulns"),
				}

				err := preflight.UpdateManifest(
					cmd.String("github-repo"),
					cmd.String("reports-branch"),
					token,
					entry,
				)
				if err != nil {
					return fmt.Errorf("updating manifest: %w", err)
				}

				fmt.Printf("Updated preflight manifest: %s/%s\n", entry.ImageName, entry.Tag)
				return nil
			},
		},
	},
}
