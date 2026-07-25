package cmd

import (
	"context"
	"os"

	"github.com/urfave/cli/v3"

	"github.com/verity-org/verity/internal/apkrepository"
)

var ciSitePublicationRestoreCommand = &cli.Command{
	Name:      "restore",
	Usage:     "Restore one exact attested prior Build Site artifact",
	ArgsUsage: "OUTPUT_DIR",
	Flags: []cli.Flag{
		&cli.StringFlag{Name: "repository", Value: os.Getenv("GITHUB_REPOSITORY")},
		&cli.Uint64Flag{Name: "run-id", Required: true},
		&cli.Uint64Flag{Name: "run-attempt", Required: true},
		&cli.StringFlag{Name: "source-sha", Required: true},
		&cli.StringFlag{Name: "artifact-digest", Required: true},
		&cli.StringFlag{Name: "manifest-digest", Required: true},
		&cli.BoolFlag{Name: "authorize-restore", Required: true},
	},
	Action: runCISitePublicationRestore,
}

func runCISitePublicationRestore(ctx context.Context, command *cli.Command) error {
	if err := requireSitePublicationArguments(command, 1); err != nil {
		return err
	}
	return apkrepository.RestorePrevious(ctx, &apkrepository.RestorePreviousOptions{
		OutputDir: command.Args().First(), Repository: command.String("repository"),
		RunID: command.Uint64("run-id"), RunAttempt: command.Uint64("run-attempt"),
		ExpectedSourceSHA: command.String("source-sha"), ExpectedArtifactDigest: command.String("artifact-digest"),
		ExpectedManifestDigest: command.String("manifest-digest"), AuthorizeRestore: command.Bool("authorize-restore"),
		Stdout: commandWriter(command),
	})
}
