package cmd

import (
	"context"

	"github.com/urfave/cli/v3"

	"github.com/verity-org/verity/internal/ci/publication"
	"github.com/verity-org/verity/internal/ci/signerlock"
	"github.com/verity-org/verity/internal/ci/sitepublication"
)

var ciSitePublicationPlanCommand = &cli.Command{
	Name:      "plan",
	Usage:     "Validate exact producer/CAS inputs and emit a compact publication plan",
	ArgsUsage: "MANIFEST",
	Flags: []cli.Flag{
		&cli.StringFlag{Name: "source-sha", Required: true},
		&cli.Uint64Flag{Name: "run-id", Required: true},
		&cli.Uint64Flag{Name: "run-attempt", Required: true},
		&cli.StringFlag{Name: "batch-id", Required: true},
		&cli.StringFlag{Name: "mode", Required: true},
		&cli.StringFlag{Name: "components", Required: true},
		&cli.StringFlag{Name: "signer-lock", Required: true},
		&cli.StringFlag{Name: "signer-source-sha", Required: true},
		&cli.StringFlag{Name: "publication-sha", Required: true},
		&cli.StringFlag{Name: "previous-manifest"},
		&cli.BoolFlag{Name: "authorize-bootstrap"},
		&cli.BoolFlag{Name: "authorize-restore"},
		&cli.StringFlag{Name: "repo-dir", Value: "."},
		&cli.StringFlag{Name: "record-output", Usage: "Write the machine record atomically to this path"},
		&cli.StringFlag{Name: "github-output", Sources: cli.EnvVars("GITHUB_OUTPUT"), Usage: "Append validated publication fields to this GitHub output file"},
	},
	Action: runCISitePublicationPlan,
}

func runCISitePublicationPlan(ctx context.Context, command *cli.Command) error {
	if err := requireSitePublicationArguments(command, 1); err != nil {
		return err
	}
	manifest, err := readPublicationManifest(command.Args().First())
	if err != nil {
		return err
	}
	components, err := readPublicationComponents(command.String("components"))
	if err != nil {
		return err
	}
	lock, err := signerlock.Load(command.String("signer-lock"))
	if err != nil {
		return err
	}
	var previous *publication.Manifest
	if path := command.String("previous-manifest"); path != "" {
		parsed, err := readPublicationManifest(path)
		if err != nil {
			return err
		}
		previous = &parsed
	}
	plan, err := sitepublication.CreatePlan(ctx, &sitepublication.PlanRequest{
		Manifest: manifest,
		ExpectedIdentity: publication.ProducerIdentity{
			SourceSHA:  publication.SourceSHA(command.String("source-sha")),
			RunID:      publication.RunID(command.Uint64("run-id")),
			RunAttempt: publication.RunAttempt(command.Uint64("run-attempt")),
			BatchID:    publication.BatchID(command.String("batch-id")),
		},
		ExpectedMode: publication.Mode(command.String("mode")), ExpectedComponents: components,
		PublicationSHA: publication.SourceSHA(command.String("publication-sha")), PreviousManifest: previous,
		SignerLock: lock, ExpectedSignerSourceSHA: command.String("signer-source-sha"),
		AuthorizeBootstrap: command.Bool("authorize-bootstrap"), AuthorizeRestore: command.Bool("authorize-restore"),
		RepositoryDir: command.String("repo-dir"),
	})
	if err != nil {
		return err
	}
	data, err := sitepublication.MarshalPlanCanonical(&plan)
	if err != nil {
		return err
	}
	if err := writeMachineRecord(command, data); err != nil {
		return err
	}
	if path := sitePublicationGitHubOutputPath(command); path != "" {
		return appendCISitePublicationPlanOutputs(path, &plan)
	}
	return nil
}
