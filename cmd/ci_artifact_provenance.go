package cmd

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"

	"github.com/verity-org/verity/internal/ci/artifactprovenance"
)

var _ = registerCIArtifactProvenanceCommand()

func registerCIArtifactProvenanceCommand() bool {
	CICommand.Commands = append(CICommand.Commands, newCIArtifactProvenanceCommand())
	return true
}

func newCIArtifactProvenanceCommand() *cli.Command {
	return &cli.Command{
		Name:  "artifact-provenance",
		Usage: "Write and verify exact GitHub Actions artifact provenance",
		Commands: []*cli.Command{
			{
				Name:   "write-manifest",
				Usage:  "Write the immutable producer identity manifest included in an artifact",
				Flags:  artifactIdentityFlags(),
				Action: runCIArtifactProvenanceWrite,
			},
			{
				Name:  "verify-download",
				Usage: "Verify repository, run attempt, source, publication, artifact name, digest, and manifest",
				Flags: append(
					artifactIdentityFlags(),
					&cli.StringFlag{Name: "artifact-digest", Required: true, Sources: cli.EnvVars("PROVENANCE_ARTIFACT_DIGEST")},
					&cli.StringFlag{Name: "token", Required: true, Sources: cli.EnvVars("PROVENANCE_API_AUTH")},
					&cli.StringFlag{Name: "api-base-url", Value: "https://api.github.com", Sources: cli.EnvVars("GITHUB_API_URL")},
				),
				Action: runCIArtifactProvenanceVerify,
			},
		},
	}
}

func artifactIdentityFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{Name: "repository", Required: true, Sources: cli.EnvVars("PROVENANCE_REPOSITORY")},
		&cli.Uint64Flag{Name: "run-id", Required: true, Sources: cli.EnvVars("PROVENANCE_RUN_ID")},
		&cli.Uint64Flag{Name: "run-attempt", Required: true, Sources: cli.EnvVars("PROVENANCE_RUN_ATTEMPT")},
		&cli.StringFlag{Name: "source-sha", Required: true, Sources: cli.EnvVars("PROVENANCE_SOURCE_SHA")},
		&cli.StringFlag{Name: "publication-id", Required: true, Sources: cli.EnvVars("PROVENANCE_PUBLICATION_ID")},
		&cli.StringFlag{Name: "artifact-name", Required: true, Sources: cli.EnvVars("PROVENANCE_ARTIFACT_NAME")},
		&cli.StringFlag{Name: "manifest", Required: true, Sources: cli.EnvVars("PROVENANCE_MANIFEST")},
	}
}

func runCIArtifactProvenanceWrite(_ context.Context, command *cli.Command) error {
	identity, err := parseArtifactIdentity(command)
	if err != nil {
		return err
	}
	if err := artifactprovenance.WriteManifest(command.String("manifest"), &identity); err != nil {
		return err
	}
	_, err = fmt.Fprintf(command.Writer, "wrote artifact provenance manifest %s\n", command.String("manifest"))
	if err != nil {
		return fmt.Errorf("write artifact provenance result: %w", err)
	}
	return nil
}

func runCIArtifactProvenanceVerify(ctx context.Context, command *cli.Command) error {
	identity, err := parseArtifactIdentity(command)
	if err != nil {
		return err
	}
	err = artifactprovenance.VerifyDownloaded(ctx, &artifactprovenance.VerifyOptions{
		Identity: identity, ArtifactDigest: command.String("artifact-digest"),
		ManifestPath: command.String("manifest"), Token: command.String("token"),
		APIBaseURL: command.String("api-base-url"),
	})
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(command.Writer, "verified exact artifact provenance")
	if err != nil {
		return fmt.Errorf("write artifact provenance verification result: %w", err)
	}
	return nil
}

func parseArtifactIdentity(command *cli.Command) (artifactprovenance.Identity, error) {
	input := artifactprovenance.IdentityInput{
		Repository: command.String("repository"), RunID: command.Uint64("run-id"),
		RunAttempt: command.Uint64("run-attempt"), SourceSHA: command.String("source-sha"),
		PublicationID: command.String("publication-id"), ArtifactName: command.String("artifact-name"),
	}
	identity, err := artifactprovenance.ParseIdentity(&input)
	if err != nil {
		return artifactprovenance.Identity{}, fmt.Errorf("parse artifact provenance identity: %w", err)
	}
	return identity, nil
}
