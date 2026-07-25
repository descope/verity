package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"

	"github.com/verity-org/verity/internal/ci/sitepublication"
)

var ciSitePublicationSignerPlanCommand = &cli.Command{
	Name:      "signer-plan",
	Usage:     "Emit the pinned pre-pull, attestation, isolated signer, and cleanup contract",
	ArgsUsage: "PLAN",
	Flags: []cli.Flag{
		&cli.StringFlag{Name: "runtime", Value: "docker"},
		&cli.StringFlag{Name: "repository", Required: true},
		&cli.StringFlag{Name: "workspace", Required: true},
		&cli.StringFlag{Name: "key-directory", Required: true},
		&cli.StringFlag{Name: "manifest", Value: "publication.json"},
		&cli.StringFlag{Name: "packages", Value: "packages"},
		&cli.StringFlag{Name: "base-apk", Value: "previous/apk"},
		&cli.StringFlag{Name: "delta-manifest", Value: "apk-delta.json"},
		&cli.StringFlag{Name: "output-apk", Value: "signed-apk"},
		&cli.StringFlag{Name: "public-key", Value: "verity.rsa.pub"},
		&cli.StringFlag{Name: "record-output", Usage: "Write the machine record atomically to this path"},
	},
	Action: runCISitePublicationSignerPlan,
}

var ciSitePublicationSignerExecuteCommand = &cli.Command{
	Name:      "signer-execute",
	Usage:     "Execute a validated signer plan without exposing the key to networked commands",
	ArgsUsage: "SIGNER_PLAN",
	Flags: []cli.Flag{
		&cli.StringFlag{Name: "record-output", Usage: "Write the machine record atomically to this path"},
		&cli.StringFlag{Name: "github-output", Sources: cli.EnvVars("GITHUB_OUTPUT"), Usage: "Append the validated output digest to this GitHub output file"},
	},
	Action: runCISitePublicationSignerExecute,
}

func runCISitePublicationSignerPlan(_ context.Context, command *cli.Command) error {
	if err := requireSitePublicationArguments(command, 1); err != nil {
		return err
	}
	plan, err := readSitePublicationPlan(command.Args().First())
	if err != nil {
		return err
	}
	signerPlan, err := sitepublication.BuildSignerPlan(&sitepublication.SignerRequest{
		Plan: plan, Runtime: command.String("runtime"), Repository: command.String("repository"),
		WorkspaceDir: command.String("workspace"), KeyDirectory: command.String("key-directory"),
		ManifestPath: command.String("manifest"), PackagesPath: command.String("packages"),
		BaseAPKPath: command.String("base-apk"), DeltaManifestPath: command.String("delta-manifest"),
		OutputAPKPath: command.String("output-apk"), PublicKeyPath: command.String("public-key"),
	})
	if err != nil {
		return err
	}
	data, err := sitepublication.MarshalSignerPlanCanonical(&signerPlan)
	if err != nil {
		return err
	}
	return writeMachineRecord(command, data)
}

func runCISitePublicationSignerExecute(ctx context.Context, command *cli.Command) error {
	if err := requireSitePublicationArguments(command, 1); err != nil {
		return err
	}
	key, err := readAPKSigningKey(command)
	if err != nil {
		return err
	}
	defer clear(key)
	data, err := os.ReadFile(command.Args().First())
	if err != nil {
		return fmt.Errorf("read signer plan: %w", err)
	}
	plan, err := sitepublication.ParseSignerPlanCanonical(data)
	if err != nil {
		return err
	}
	result, err := sitepublication.ExecuteSigner(ctx, &plan, key, nil)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(&result)
	if err != nil {
		return err
	}
	if err := writeMachineRecord(command, encoded); err != nil {
		return err
	}
	if path := sitePublicationGitHubOutputPath(command); path != "" {
		return appendCISitePublicationSignerOutputs(path, result)
	}
	return nil
}
