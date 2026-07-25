package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"

	"github.com/urfave/cli/v3"

	"github.com/verity-org/verity/internal/apkrepository"
	"github.com/verity-org/verity/internal/ci/publication"
	"github.com/verity-org/verity/internal/ci/sitepublication"
)

var ciSitePublicationSignCommand = &cli.Command{
	Name:      "sign",
	Usage:     "Run the existing APK snapshot/delta logic inside the isolated signer",
	ArgsUsage: "MANIFEST",
	Flags: []cli.Flag{
		&cli.StringFlag{Name: "input-root", Required: true},
		&cli.StringFlag{Name: "publication-plan-digest", Required: true},
		&cli.StringFlag{Name: "manifest-digest", Required: true},
		&cli.StringFlag{Name: "input-digest", Required: true},
		&cli.StringFlag{Name: "input-authorization", Required: true},
		&cli.StringFlag{Name: "packages", Required: true},
		&cli.StringFlag{Name: "base-apk"},
		&cli.StringFlag{Name: "delta-manifest"},
		&cli.StringFlag{Name: "output", Required: true},
		&cli.StringFlag{Name: "public-key", Required: true},
		&cli.StringFlag{Name: "key-name", Value: "verity.rsa"},
	},
	Action: runCISitePublicationSign,
}

type ciSitePublicationSignRequest struct {
	authorization         sitepublication.SignerInputAuthorization
	inputDigest           publication.Digest
	publicationPlanDigest publication.Digest
	manifestDigest        publication.Digest
	mode                  publication.Mode
	inputRoot             string
	manifestPath          string
	packagesPath          string
	baseAPKPath           string
	deltaManifestPath     string
	outputPath            string
	publicKeyPath         string
	keyName               string
}

func runCISitePublicationSign(ctx context.Context, command *cli.Command) error {
	request, err := newCISitePublicationSignRequest(command)
	if err != nil {
		return err
	}
	key, err := readAPKSigningKey(command)
	if err != nil {
		return err
	}
	defer clear(key)
	if err := sitepublication.VerifySignerInputs(
		request.inputRoot,
		&request.authorization,
		request.inputDigest,
		request.publicationPlanDigest,
		request.manifestDigest,
	); err != nil {
		return err
	}
	stderr := commandErrorWriter(command)
	if err := request.sign(ctx, key, stderr); err != nil {
		return err
	}
	if err := apkrepository.Validate(ctx, &apkrepository.ValidateOptions{
		RepositoryDir: request.outputPath, RequireSignature: true, VerifyCrypto: true,
		Stdout: stderr, Stderr: stderr,
	}); err != nil {
		return fmt.Errorf("validate signed APK repository: %w", err)
	}
	digest, err := apkrepository.RepositoryDigest(request.outputPath)
	if err != nil {
		return fmt.Errorf("digest signed APK repository: %w", err)
	}
	encoded, err := json.Marshal(&sitepublication.SignerOperationResult{
		OutputPath: request.outputPath, OutputDigest: publication.Digest(digest),
	})
	if err != nil {
		return fmt.Errorf("encode signer result: %w", err)
	}
	return writeMachineRecord(command, encoded)
}

func newCISitePublicationSignRequest(command *cli.Command) (*ciSitePublicationSignRequest, error) {
	if err := requireSitePublicationArguments(command, 1); err != nil {
		return nil, err
	}
	request := &ciSitePublicationSignRequest{
		inputDigest:           publication.Digest(command.String("input-digest")),
		publicationPlanDigest: publication.Digest(command.String("publication-plan-digest")),
		manifestDigest:        publication.Digest(command.String("manifest-digest")),
		inputRoot:             command.String("input-root"),
		manifestPath:          command.Args().First(),
		packagesPath:          command.String("packages"),
		baseAPKPath:           command.String("base-apk"),
		deltaManifestPath:     command.String("delta-manifest"),
		outputPath:            command.String("output"),
		publicKeyPath:         command.String("public-key"),
		keyName:               command.String("key-name"),
	}
	authorization, err := sitepublication.VerifySignerInputsBase64(
		request.inputRoot,
		command.String("input-authorization"),
		request.inputDigest,
		request.publicationPlanDigest,
		request.manifestDigest,
	)
	if err != nil {
		return nil, err
	}
	request.authorization = authorization
	if request.manifestPath != filepath.Join(request.inputRoot, filepath.FromSlash(authorization.ManifestPath)) ||
		request.packagesPath != filepath.Join(request.inputRoot, filepath.FromSlash(authorization.PackagesPath)) ||
		request.publicKeyPath != filepath.Join(request.inputRoot, filepath.FromSlash(authorization.PublicKeyPath)) {
		return nil, fmt.Errorf("%w: signer command paths do not match authorization", sitepublication.ErrInvalidSignerPlan)
	}
	manifest, err := readPublicationManifest(request.manifestPath)
	if err != nil {
		return nil, err
	}
	request.mode = manifest.Mode
	return request, nil
}

func (request *ciSitePublicationSignRequest) sign(ctx context.Context, key []byte, stderr io.Writer) error {
	switch request.mode {
	case publication.ModeBootstrap, publication.ModeSnapshot:
		return apkrepository.Snapshot(ctx, &apkrepository.SnapshotOptions{
			OutputDir: request.outputPath, KeyName: request.keyName,
			PublicKeyPath: request.publicKeyPath, Sources: []string{request.packagesPath},
			PrivateKeyPEM: key, Stdout: stderr, Stderr: stderr,
		})
	case publication.ModeDelta:
		if request.baseAPKPath == "" || request.deltaManifestPath == "" {
			return fmt.Errorf("%w: delta signing requires --base-apk and --delta-manifest", errInvalidSitePublicationArguments)
		}
		if request.baseAPKPath != filepath.Join(request.inputRoot, filepath.FromSlash(request.authorization.BaseAPKPath)) ||
			request.deltaManifestPath != filepath.Join(request.inputRoot, filepath.FromSlash(request.authorization.DeltaManifestPath)) {
			return fmt.Errorf("%w: delta paths do not match authorization", sitepublication.ErrInvalidSignerPlan)
		}
		return apkrepository.ApplyDelta(ctx, &apkrepository.DeltaOptions{
			BaseDir: request.baseAPKPath, PackageDir: request.packagesPath,
			ManifestPath: request.deltaManifestPath, OutputDir: request.outputPath,
			KeyName: request.keyName, PrivateKeyPEM: key,
			Stdout: stderr, Stderr: stderr,
		})
	case publication.ModeRestore:
		return sitepublication.ErrUnsupportedSignMode
	default:
		return fmt.Errorf("%w: %q", sitepublication.ErrUnsupportedSignMode, request.mode)
	}
}
