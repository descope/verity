package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"

	"github.com/verity-org/verity/internal/ci/publication"
)

var errInvalidPublicationArguments = errors.New("invalid publication arguments")

var ciPublicationCommand = &cli.Command{
	Name:  "publication",
	Usage: "Validate canonical publication manifests and state transitions",
	Commands: []*cli.Command{
		ciPublicationComposeCommand,
		ciPublicationDigestCommand,
		ciPublicationValidateCommand,
	},
}

var ciPublicationDigestCommand = &cli.Command{
	Name:      "digest",
	Usage:     "Print the canonical SHA-256 identity of a manifest artifact",
	ArgsUsage: "MANIFEST",
	Action: func(_ context.Context, command *cli.Command) error {
		if err := requirePublicationArguments(command, 1); err != nil {
			return err
		}
		manifest, err := readPublicationManifest(command.Args().First())
		if err != nil {
			return err
		}
		digest, err := publication.DigestManifest(&manifest)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(os.Stdout, digest)
		if err != nil {
			return fmt.Errorf("write publication digest: %w", err)
		}
		return nil
	},
}

var ciPublicationValidateCommand = &cli.Command{
	Name:      "validate",
	Usage:     "Validate exact producer identity, ancestry, completeness, and previous-state CAS",
	ArgsUsage: "MANIFEST",
	Flags: []cli.Flag{
		&cli.StringFlag{Name: "source-sha", Required: true},
		&cli.Uint64Flag{Name: "run-id", Required: true},
		&cli.Uint64Flag{Name: "run-attempt", Required: true},
		&cli.StringFlag{Name: "batch-id", Required: true},
		&cli.StringFlag{Name: "mode", Required: true},
		&cli.StringFlag{Name: "components", Required: true, Usage: "Canonical expected component array artifact"},
		&cli.StringFlag{Name: "signer-digest", Required: true},
		&cli.StringFlag{Name: "publication-sha", Required: true},
		&cli.StringFlag{Name: "previous-manifest", Usage: "Canonical current publication manifest"},
		&cli.BoolFlag{Name: "authorize-bootstrap"},
		&cli.BoolFlag{Name: "authorize-restore"},
		&cli.StringFlag{Name: "repo-dir", Value: "."},
	},
	Action: runCIPublicationValidate,
}

var _ = registerCIPublicationCommand()

func registerCIPublicationCommand() bool {
	CICommand.Commands = append(CICommand.Commands, ciPublicationCommand)
	return true
}

func runCIPublicationValidate(ctx context.Context, command *cli.Command) error {
	if err := requirePublicationArguments(command, 1); err != nil {
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
	var previous *publication.Manifest
	if path := command.String("previous-manifest"); path != "" {
		parsed, err := readPublicationManifest(path)
		if err != nil {
			return err
		}
		previous = &parsed
	}
	err = publication.Validate(ctx, &manifest, &publication.ValidationOptions{
		ExpectedIdentity: publication.ProducerIdentity{
			SourceSHA:  publication.SourceSHA(command.String("source-sha")),
			RunID:      publication.RunID(command.Uint64("run-id")),
			RunAttempt: publication.RunAttempt(command.Uint64("run-attempt")),
			BatchID:    publication.BatchID(command.String("batch-id")),
		},
		ExpectedMode:         publication.Mode(command.String("mode")),
		ExpectedComponents:   components,
		ExpectedSignerDigest: publication.Digest(command.String("signer-digest")),
		PublicationSHA:       publication.SourceSHA(command.String("publication-sha")),
		PreviousManifest:     previous,
		AuthorizeBootstrap:   command.Bool("authorize-bootstrap"),
		AuthorizeRestore:     command.Bool("authorize-restore"),
		RepositoryDir:        command.String("repo-dir"),
	})
	if err != nil {
		return err
	}
	digest, err := publication.DigestManifest(&manifest)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(os.Stdout, "validated %s\n", digest)
	if err != nil {
		return fmt.Errorf("write publication validation: %w", err)
	}
	return nil
}

func readPublicationManifest(path string) (publication.Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return publication.Manifest{}, fmt.Errorf("read publication manifest %q: %w", path, err)
	}
	manifest, err := publication.ParseCanonical(data)
	if err != nil {
		return publication.Manifest{}, fmt.Errorf("parse publication manifest %q: %w", path, err)
	}
	return manifest, nil
}

func readPublicationComponents(path string) ([]publication.Component, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read publication components %q: %w", path, err)
	}
	components, err := publication.ParseComponentsCanonical(data)
	if err != nil {
		return nil, fmt.Errorf("parse publication components %q: %w", path, err)
	}
	return components, nil
}

func requirePublicationArguments(command *cli.Command, count int) error {
	if command.Args().Len() != count {
		return fmt.Errorf("%w: %s expects %d positional arguments, received %d", errInvalidPublicationArguments, command.FullName(), count, command.Args().Len())
	}
	return nil
}
