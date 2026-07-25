package cmd

import (
	"context"
	"encoding/json"

	"github.com/urfave/cli/v3"

	"github.com/verity-org/verity/internal/ci/sitepublication"
)

var ciSitePublicationAssembleCommand = &cli.Command{
	Name:      "assemble",
	Usage:     "Deterministically overlay a full site while preserving unlisted bytes",
	ArgsUsage: "MANIFEST PLAN",
	Flags: []cli.Flag{
		&cli.StringFlag{Name: "base"},
		&cli.StringFlag{Name: "signed-apk", Required: true},
		&cli.StringFlag{Name: "output", Required: true},
		&cli.StringSliceFlag{Name: "overlay", Usage: "Declared NAME=SOURCE tree overlay"},
		&cli.StringFlag{Name: "record-output", Usage: "Write the machine record atomically to this path"},
	},
	Action: runCISitePublicationAssemble,
}

func runCISitePublicationAssemble(ctx context.Context, command *cli.Command) error {
	if err := requireSitePublicationArguments(command, 2); err != nil {
		return err
	}
	manifest, err := readPublicationManifest(command.Args().Get(0))
	if err != nil {
		return err
	}
	plan, err := readSitePublicationPlan(command.Args().Get(1))
	if err != nil {
		return err
	}
	overlays, err := parseOverlays(command.StringSlice("overlay"))
	if err != nil {
		return err
	}
	result, err := sitepublication.AssembleSite(ctx, &sitepublication.AssembleRequest{
		Plan: plan, Manifest: manifest, BaseDir: command.String("base"),
		SignedAPKDir: command.String("signed-apk"), OutputDir: command.String("output"), Overlays: overlays,
	})
	if err != nil {
		return err
	}
	data, err := json.Marshal(&result)
	if err != nil {
		return err
	}
	return writeMachineRecord(command, data)
}
