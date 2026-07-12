package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"

	"github.com/verity-org/verity/internal/integer/melange"
)

var (
	integerMelangePrepare = melange.Prepare
	integerMelangeBuild   = melange.Build
)

var integerMelangeCmd = &cli.Command{
	Name:  "melange",
	Usage: "Prepare and build bespoke packages",
	Commands: []*cli.Command{
		{
			Name:  "prepare",
			Usage: "Verify and stage a local recipe with an ephemeral signing key",
			Flags: append(integerMelangeSpecFlags(), &cli.StringFlag{
				Name:  "github-output",
				Usage: "Append build metadata to a GitHub Actions output file",
			}),
			Action: integerMelangePrepareAction,
		},
		{
			Name:  "build",
			Usage: "Build and sign a staged bespoke package repository",
			Flags: append(integerMelangeSpecFlags(),
				&cli.StringFlag{
					Name:     "arch",
					Usage:    "Target package architecture",
					Required: true,
				},
				&cli.BoolFlag{
					Name:  "staged",
					Usage: "Reuse previously staged recipes and signing key",
				},
			),
			Action: integerMelangeBuildAction,
		},
	},
}

func integerMelangeSpecFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:  "root",
			Usage: "Repository root",
			Value: ".",
		},
		&cli.StringFlag{
			Name:     "image",
			Usage:    "Image name",
			Required: true,
		},
		&cli.StringFlag{
			Name:     "version",
			Usage:    "Image version",
			Required: true,
		},
		&cli.StringFlag{
			Name:  "type",
			Usage: "Image type",
			Value: "default",
		},
	}
}

func integerMelangePrepareAction(ctx context.Context, cmd *cli.Command) error {
	paths, spec, err := integerMelangeOptions(cmd)
	if err != nil {
		return err
	}
	if err := integerMelangePrepare(ctx, &melange.BuildOptions{Paths: paths, Spec: spec}); err != nil {
		return fmt.Errorf("prepare bespoke package: %w", err)
	}
	if outputPath := cmd.String("github-output"); outputPath != "" {
		if err := writeIntegerMelangeGitHubOutput(outputPath, spec); err != nil {
			return err
		}
	}
	return nil
}

func integerMelangeBuildAction(ctx context.Context, cmd *cli.Command) error {
	paths, spec, err := integerMelangeOptions(cmd)
	if err != nil {
		return err
	}
	arch, err := melange.ParseArchitecture(cmd.String("arch"))
	if err != nil {
		return err
	}
	options := melange.BuildOptions{
		Paths:  paths,
		Spec:   spec,
		Arch:   arch,
		Staged: cmd.Bool("staged"),
	}
	if err := integerMelangeBuild(ctx, &options); err != nil {
		return fmt.Errorf("build bespoke package: %w", err)
	}
	return nil
}

func integerMelangeOptions(cmd *cli.Command) (melange.Paths, melange.Spec, error) {
	paths := melange.DefaultPaths(cmd.String("root"))
	spec, err := melange.ResolveSpec(paths.ImagesDir, cmd.String("image"), cmd.String("version"), cmd.String("type"))
	if err != nil {
		return melange.Paths{}, melange.Spec{}, fmt.Errorf("resolve bespoke package: %w", err)
	}
	return paths, spec, nil
}

func writeIntegerMelangeGitHubOutput(path string, spec melange.Spec) error {
	output, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open GitHub output: %w", err)
	}
	if err := melange.WriteGitHubOutput(output, spec); err != nil {
		_ = output.Close()
		return fmt.Errorf("write GitHub output: %w", err)
	}
	if err := output.Close(); err != nil {
		return fmt.Errorf("close GitHub output: %w", err)
	}
	return nil
}
