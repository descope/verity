package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/verity-org/verity/internal/ci/publication"
	"github.com/verity-org/verity/internal/ci/repositoryops"
	"github.com/verity-org/verity/internal/ci/signerlock"
)

var ciPublicationComposeCommand = &cli.Command{
	Name:  "compose",
	Usage: "Materialize canonical publication and component manifests from exact producer artifacts",
	Flags: []cli.Flag{
		&cli.StringFlag{Name: "source-sha", Required: true},
		&cli.Uint64Flag{Name: "run-id", Required: true},
		&cli.Uint64Flag{Name: "run-attempt", Required: true},
		&cli.StringFlag{Name: "batch-id", Required: true},
		&cli.StringFlag{Name: "mode", Required: true},
		&cli.StringFlag{Name: "previous-manifest"},
		&cli.StringFlag{Name: "previous-manifest-digest"},
		&cli.StringFlag{Name: "signer-digest", Usage: "Raw signer image digest; mutually exclusive with --signer-lock"},
		&cli.StringFlag{Name: "signer-lock", Usage: "Validated signer lock JSON; mutually exclusive with --signer-digest"},
		&cli.StringFlag{Name: "publication-sha", Required: true},
		&cli.StringFlag{Name: "apk-operations", Usage: "Canonical publication APK operation array"},
		&cli.StringFlag{Name: "apk-delta", Usage: "Exact APK repository delta manifest"},
		&cli.StringFlag{Name: "signing-key-state", Usage: "Validated signing key state JSON and committed public key"},
		&cli.Uint64Flag{Name: "signing-key-epoch"},
		&cli.StringFlag{Name: "active-signing-key-fingerprint"},
		&cli.StringSliceFlag{Name: "trusted-signing-key-fingerprint"},
		&cli.StringSliceFlag{Name: "revoked-signing-key-fingerprint"},
		&cli.StringSliceFlag{Name: "producer-manifest", Required: true, Usage: "NAME=ARTIFACT_NAME=ARTIFACT_DIGEST=PATH (repeatable)"},
		&cli.StringFlag{Name: "publication-output", Required: true},
		&cli.StringFlag{Name: "components-output", Required: true},
		&cli.StringFlag{Name: "github-output", Sources: cli.EnvVars("GITHUB_OUTPUT")},
		&cli.BoolFlag{Name: "authorize-bootstrap"},
		&cli.BoolFlag{Name: "authorize-restore"},
		&cli.StringFlag{Name: "repo-dir", Value: "."},
	},
	Action: runCIPublicationCompose,
}

var errOptionalFileAbsent = errors.New("optional file is absent")

type resolvedSigner struct {
	Digest    publication.Digest
	SourceSHA publication.SourceSHA
}

func runCIPublicationCompose(ctx context.Context, command *cli.Command) error {
	signer, err := resolveSigner(command.String("signer-digest"), command.String("signer-lock"))
	if err != nil {
		return err
	}
	signingKey, err := resolvePublicationSigningKeyState(
		command.String("signing-key-state"), command.String("repo-dir"),
		command.Uint64("signing-key-epoch"), command.String("active-signing-key-fingerprint"),
		command.StringSlice("trusted-signing-key-fingerprint"), command.StringSlice("revoked-signing-key-fingerprint"),
	)
	if err != nil {
		return err
	}
	producers, err := readProducerManifestInputs(command.StringSlice("producer-manifest"))
	if err != nil {
		return err
	}
	previous, err := optionalPublicationManifest(command.String("previous-manifest"))
	if errors.Is(err, errOptionalFileAbsent) {
		previous = nil
		err = nil
	}
	if err != nil {
		return err
	}
	operations, err := optionalAPKOperations(command.String("apk-operations"))
	if errors.Is(err, errOptionalFileAbsent) {
		operations = nil
		err = nil
	}
	if err != nil {
		return err
	}
	delta, err := optionalFile(command.String("apk-delta"))
	if errors.Is(err, errOptionalFileAbsent) {
		delta = nil
		err = nil
	}
	if err != nil {
		return err
	}
	result, err := publication.Compose(ctx, &publication.ComposeRequest{
		Identity: publication.ProducerIdentity{
			SourceSHA: publication.SourceSHA(command.String("source-sha")),
			RunID:     publication.RunID(command.Uint64("run-id")), RunAttempt: publication.RunAttempt(command.Uint64("run-attempt")),
			BatchID: publication.BatchID(command.String("batch-id")),
		},
		Mode: publication.Mode(command.String("mode")), PreviousManifest: previous,
		PreviousManifestDigest: publication.Digest(command.String("previous-manifest-digest")),
		SignerDigest:           signer.Digest,
		PublicationSHA:         publication.SourceSHA(command.String("publication-sha")),
		APKOperations:          operations, APKDelta: delta,
		SigningKeyEpoch:               signingKey.Epoch,
		ActiveSigningKeyFingerprint:   signingKey.ActiveFingerprint,
		TrustedSigningKeyFingerprints: signingKey.TrustedFingerprints,
		RevokedSigningKeyFingerprints: signingKey.RevokedFingerprints,
		AuthorizeBootstrap:            command.Bool("authorize-bootstrap"), AuthorizeRestore: command.Bool("authorize-restore"),
		RepositoryDir: command.String("repo-dir"), Producers: producers,
	})
	if err != nil {
		return err
	}
	if err := publication.WriteComposeOutputs(command.String("publication-output"), command.String("components-output"), &result); err != nil {
		return err
	}
	if outputPath := command.String("github-output"); outputPath != "" {
		values := []repositoryops.WorkflowValue{{Name: "signer_digest", Value: string(signer.Digest)}}
		if signer.SourceSHA != "" {
			values = append(values, repositoryops.WorkflowValue{Name: "signer_source_sha", Value: string(signer.SourceSHA)})
		}
		return repositoryops.AppendWorkflowValues(outputPath, values)
	}
	return nil
}

func resolveSigner(rawDigest, lockPath string) (resolvedSigner, error) {
	if rawDigest != "" && lockPath != "" {
		return resolvedSigner{}, fmt.Errorf("%w: --signer-digest and --signer-lock are mutually exclusive", errInvalidPublicationArguments)
	}
	if lockPath != "" {
		lock, err := signerlock.Load(lockPath)
		if err != nil {
			return resolvedSigner{}, fmt.Errorf("load signer lock %q: %w", lockPath, err)
		}
		return resolvedSigner{
			Digest:    publication.Digest(lock.Digest),
			SourceSHA: publication.SourceSHA(lock.SourceSHA),
		}, nil
	}
	if rawDigest == "" {
		return resolvedSigner{}, fmt.Errorf("%w: one of --signer-digest or --signer-lock is required", errInvalidPublicationArguments)
	}
	return resolvedSigner{Digest: publication.Digest(rawDigest)}, nil
}

func readProducerManifestInputs(values []string) ([]publication.ProducerManifestInput, error) {
	inputs := make([]publication.ProducerManifestInput, 0, len(values))
	for _, value := range values {
		parts := strings.SplitN(value, "=", 4)
		if len(parts) != 4 || parts[0] == "" || parts[1] == "" || parts[2] == "" || parts[3] == "" {
			return nil, fmt.Errorf("%w: producer manifest must be NAME=ARTIFACT_NAME=ARTIFACT_DIGEST=PATH", errInvalidPublicationArguments)
		}
		data, err := os.ReadFile(parts[3])
		if err != nil {
			return nil, fmt.Errorf("read producer manifest %q: %w", parts[3], err)
		}
		inputs = append(inputs, publication.ProducerManifestInput{
			Name: parts[0], ArtifactName: parts[1], ArtifactDigest: publication.Digest(parts[2]), Data: data,
		})
	}
	return inputs, nil
}

func optionalPublicationManifest(path string) (*publication.Manifest, error) {
	if path == "" {
		return nil, errOptionalFileAbsent
	}
	manifest, err := readPublicationManifest(path)
	if err != nil {
		return nil, err
	}
	return &manifest, nil
}

func optionalAPKOperations(path string) ([]publication.APKOperation, error) {
	data, err := optionalFile(path)
	if errors.Is(err, errOptionalFileAbsent) {
		return nil, errOptionalFileAbsent
	}
	if err != nil {
		return nil, err
	}
	operations, err := publication.ParseAPKOperationsCanonical(data)
	if err != nil {
		return nil, fmt.Errorf("parse APK operations %q: %w", path, err)
	}
	return operations, nil
}

func optionalFile(path string) ([]byte, error) {
	if path == "" {
		return nil, errOptionalFileAbsent
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %q: %w", path, err)
	}
	return data, nil
}
