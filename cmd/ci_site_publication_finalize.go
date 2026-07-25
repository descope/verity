package cmd

import (
	"context"

	"github.com/urfave/cli/v3"

	"github.com/verity-org/verity/internal/ci/publication"
	"github.com/verity-org/verity/internal/ci/sitepublication"
)

var ciSitePublicationFinalizeCommand = &cli.Command{
	Name:      "finalize",
	Usage:     "Recheck CAS, pack deterministic bytes, and emit attestation/deploy eligibility",
	ArgsUsage: "PLAN SITE_DIR ARCHIVE",
	Flags: []cli.Flag{
		&cli.StringFlag{Name: "plan-digest", Required: true},
		&cli.StringFlag{Name: "current-manifest"},
		&cli.StringFlag{Name: "record-output", Usage: "Write the machine record atomically to this path"},
		&cli.StringFlag{Name: "github-output", Sources: cli.EnvVars("GITHUB_OUTPUT"), Usage: "Append validated finalization fields to this GitHub output file"},
	},
	Action: runCISitePublicationFinalize,
}

func runCISitePublicationFinalize(ctx context.Context, command *cli.Command) error {
	if err := requireSitePublicationArguments(command, 3); err != nil {
		return err
	}
	plan, err := readSitePublicationPlan(command.Args().Get(0))
	if err != nil {
		return err
	}
	var current *publication.Manifest
	if path := command.String("current-manifest"); path != "" {
		parsed, err := readPublicationManifest(path)
		if err != nil {
			return err
		}
		current = &parsed
	}
	finalPlan, err := sitepublication.FinalizePublication(ctx, &sitepublication.FinalizeRequest{
		Plan: plan, ExpectedPlanDigest: publication.Digest(command.String("plan-digest")),
		SiteDir: command.Args().Get(1), ArchivePath: command.Args().Get(2), CurrentManifest: current,
	})
	if err != nil {
		return err
	}
	data, err := sitepublication.MarshalFinalPlanCanonical(&finalPlan)
	if err != nil {
		return err
	}
	if err := writeMachineRecord(command, data); err != nil {
		return err
	}
	if path := sitePublicationGitHubOutputPath(command); path != "" {
		return appendCISitePublicationFinalizeOutputs(path, &finalPlan)
	}
	return nil
}
