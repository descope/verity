package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"

	"github.com/verity-org/verity/internal/apkrepository"
)

var errInvalidAPKRepositoryArguments = errors.New("invalid APK repository arguments")

var ciAPKRepositoryCommand = &cli.Command{
	Name:  "apk-repository",
	Usage: "Publish and validate the signed APK repository",
	Commands: []*cli.Command{
		ciAPKRepositoryAssembleCommand,
		ciAPKRepositorySnapshotCommand,
		ciAPKRepositoryDeltaCommand,
		ciAPKRepositoryValidateCommand,
		ciAPKRepositoryDownloadApprovedCommand,
		ciAPKRepositoryRestorePreviousCommand,
		ciAPKRepositorySelectCommand,
	},
}

var ciAPKRepositoryAssembleCommand = &cli.Command{
	Name:      "assemble",
	Usage:     "Assemble APK packages into signed architecture repositories",
	ArgsUsage: "[SOURCE_DIR ...]",
	Flags:     apkRepositoryBuildFlags(),
	Action: func(ctx context.Context, command *cli.Command) error {
		key, err := readAPKSigningKey(command)
		if err != nil {
			return err
		}
		defer clear(key)
		return apkrepository.Assemble(ctx, apkRepositoryBuildOptions(command, key))
	},
}

var ciAPKRepositorySnapshotCommand = &cli.Command{
	Name:      "snapshot",
	Usage:     "Publish a complete signed x86_64/aarch64 APK snapshot",
	ArgsUsage: "[SOURCE_DIR ...]",
	Flags:     apkRepositoryBuildFlags(),
	Action: func(ctx context.Context, command *cli.Command) error {
		key, err := readAPKSigningKey(command)
		if err != nil {
			return err
		}
		defer clear(key)
		return apkrepository.Snapshot(ctx, apkRepositoryBuildOptions(command, key))
	},
}

func apkRepositoryBuildFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{Name: "output", Aliases: []string{"o"}, Value: "site/dist/apk", Usage: "Repository output directory"},
		&cli.StringFlag{Name: "key-name", Value: "verity.rsa", Usage: "Published RSA key basename"},
		&cli.StringFlag{Name: "public-key", Usage: "Committed public key path"},
	}
}

func apkRepositoryBuildOptions(command *cli.Command, key []byte) *apkrepository.AssembleOptions {
	return &apkrepository.AssembleOptions{
		OutputDir:     command.String("output"),
		KeyName:       command.String("key-name"),
		PublicKeyPath: command.String("public-key"),
		Sources:       command.Args().Slice(),
		PrivateKeyPEM: key,
		Stdout:        os.Stdout,
		Stderr:        os.Stderr,
	}
}

var ciAPKRepositoryDeltaCommand = &cli.Command{
	Name:      "delta",
	Usage:     "Apply a manifest-authorized APK delta to a signed base repository",
	ArgsUsage: "BASE_DIR PACKAGE_DIR",
	Flags: []cli.Flag{
		&cli.StringFlag{Name: "manifest", Required: true, Usage: "Canonical APK delta manifest path"},
		&cli.StringFlag{Name: "output", Aliases: []string{"o"}, Value: "site/dist/apk", Usage: "Repository output directory"},
		&cli.StringFlag{Name: "key-name", Value: "verity.rsa", Usage: "Published RSA key basename"},
	},
	Action: func(ctx context.Context, command *cli.Command) error {
		if err := requireAPKRepositoryArguments(command, 2); err != nil {
			return err
		}
		key, err := readAPKSigningKey(command)
		if err != nil {
			return err
		}
		defer clear(key)
		return apkrepository.ApplyDelta(ctx, &apkrepository.DeltaOptions{
			BaseDir:       command.Args().Get(0),
			PackageDir:    command.Args().Get(1),
			ManifestPath:  command.String("manifest"),
			OutputDir:     command.String("output"),
			KeyName:       command.String("key-name"),
			PrivateKeyPEM: key,
			Stdout:        os.Stdout,
			Stderr:        os.Stderr,
		})
	},
}

var ciAPKRepositoryValidateCommand = &cli.Command{
	Name:      "validate",
	Usage:     "Validate repository layout and signatures",
	ArgsUsage: "REPO_DIR",
	Flags: []cli.Flag{
		&cli.BoolFlag{Name: "require-signature", Usage: "Require RSA256 index signatures and matching root keys"},
		&cli.BoolFlag{Name: "verify-crypto", Usage: "Verify package signatures and fresh-client repository trust"},
	},
	Action: func(ctx context.Context, command *cli.Command) error {
		if err := requireAPKRepositoryArguments(command, 1); err != nil {
			return err
		}
		return apkrepository.Validate(ctx, &apkrepository.ValidateOptions{
			RepositoryDir:    command.Args().First(),
			RequireSignature: command.Bool("require-signature"),
			VerifyCrypto:     command.Bool("verify-crypto"),
			Stdout:           os.Stdout,
			Stderr:           os.Stderr,
		})
	},
}

var ciAPKRepositoryDownloadApprovedCommand = &cli.Command{
	Name:      "download-approved",
	Usage:     "Download and attest packages from an exact Integer batch",
	ArgsUsage: "RUN_ID-RUN_ATTEMPT OUTPUT_DIR",
	Action: func(ctx context.Context, command *cli.Command) error {
		if err := requireAPKRepositoryArguments(command, 2); err != nil {
			return err
		}
		return apkrepository.DownloadApproved(ctx, &apkrepository.DownloadApprovedOptions{
			BatchID:    command.Args().Get(0),
			OutputDir:  command.Args().Get(1),
			Repository: os.Getenv("GITHUB_REPOSITORY"),
			Stdout:     os.Stdout,
		})
	},
}

var ciAPKRepositoryRestorePreviousCommand = &cli.Command{
	Name:      "restore-previous",
	Usage:     "Restore the latest successful main-branch Pages APK repository",
	ArgsUsage: "OUTPUT_DIR",
	Action: func(ctx context.Context, command *cli.Command) error {
		if err := requireAPKRepositoryArguments(command, 1); err != nil {
			return err
		}
		return apkrepository.RestorePrevious(ctx, &apkrepository.RestorePreviousOptions{
			OutputDir:  command.Args().First(),
			Repository: os.Getenv("GITHUB_REPOSITORY"),
			Stdout:     os.Stdout,
		})
	},
}

var ciAPKRepositorySelectCommand = &cli.Command{
	Name:      "select",
	Usage:     "Preserve published bytes when package and trust state are unchanged",
	ArgsUsage: "CANDIDATE_DIR PREVIOUS_DIR OUTPUT_DIR",
	Action: func(ctx context.Context, command *cli.Command) error {
		if err := requireAPKRepositoryArguments(command, 3); err != nil {
			return err
		}
		return apkrepository.Select(ctx, &apkrepository.SelectOptions{
			CandidateDir: command.Args().Get(0),
			PreviousDir:  command.Args().Get(1),
			OutputDir:    command.Args().Get(2),
			Stdout:       os.Stdout,
		})
	},
}

func requireAPKRepositoryArguments(command *cli.Command, count int) error {
	if command.Args().Len() != count {
		return fmt.Errorf("%w: %s expects %d positional arguments, received %d", errInvalidAPKRepositoryArguments, command.FullName(), count, command.Args().Len())
	}
	return nil
}
