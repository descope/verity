package patchimage

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/verity-org/verity/internal/ci/repositoryops"
)

func newCreateManifestCommand() *cli.Command {
	return &cli.Command{
		Name: "create-manifest",
		Flags: []cli.Flag{
			imageNameFlag(), sourceTagFlag(),
			&cli.StringFlag{Name: "staging-registry", Required: true, Sources: cli.EnvVars("STAGING_REGISTRY")},
			&cli.StringFlag{Name: "platforms", Required: true, Sources: cli.EnvVars("PLATFORMS")},
			outputFlag(),
		},
		Action: func(ctx context.Context, command *cli.Command) error {
			plan, err := (ManifestService{Stdout: os.Stdout, Stderr: os.Stderr}).Create(ctx, ManifestPlanInput{
				ImageName: command.String("image-name"), SourceTag: command.String("source-tag"),
				StagingRegistry: command.String("staging-registry"), Platforms: command.String("platforms"),
			})
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(os.Stdout, "✓ Created manifest: %s\n", plan.ManifestTag); err != nil {
				return fmt.Errorf("print manifest result: %w", err)
			}
			return appendWorkflowOutputs(command.String("github-output"), repositoryops.WorkflowValue{Name: "manifest-tag", Value: plan.ManifestTag})
		},
	}
}

func newCompareReportsCommand() *cli.Command {
	return &cli.Command{
		Name: "compare-reports",
		Flags: []cli.Flag{
			imageNameFlag(), sourceTagFlag(),
			&cli.StringFlag{Name: "target-registry", Required: true, Sources: cli.EnvVars("TARGET_REGISTRY")},
			&cli.StringFlag{Name: "current-report", Value: "post.json"},
			&cli.StringFlag{Name: "previous-report", Value: "prev-post.json"},
			&cli.BoolFlag{Name: "previous-existed", Sources: cli.EnvVars("PREVIOUS_EXISTED")},
			outputFlag(),
		},
		Action: func(ctx context.Context, command *cli.Command) error {
			current, err := ReadTrivyReport(command.String("current-report"))
			if err != nil {
				return err
			}
			previousExisted := command.Bool("previous-existed")
			previous, err := ReadTrivyReport(command.String("previous-report"))
			if err != nil {
				previousExisted = false
				previous = TrivySummary{BySeverity: baseSeverityCounts()}
			}
			finalTag := command.String("target-registry") + "/" + command.String("image-name") + ":" + command.String("source-tag")
			changed := ShouldPublish(&CompareInput{
				PreviousExisted: previousExisted, TargetExists: (ManifestService{}).Exists(ctx, finalTag),
				Current: current, Previous: previous,
			})
			if _, err := fmt.Fprintf(os.Stdout, "publish changed=%t\n", changed); err != nil {
				return fmt.Errorf("print comparison result: %w", err)
			}
			return appendWorkflowOutputs(command.String("github-output"), repositoryops.WorkflowValue{Name: "changed", Value: strconv.FormatBool(changed)})
		},
	}
}

func newCraneCommand() *cli.Command {
	return &cli.Command{Name: "crane", Commands: []*cli.Command{newCraneCopyCommand()}}
}

func newCraneCopyCommand() *cli.Command {
	return &cli.Command{
		Name: "copy",
		Flags: []cli.Flag{
			imageNameFlag(), sourceTagFlag(),
			&cli.StringFlag{Name: "target-registry", Required: true, Sources: cli.EnvVars("TARGET_REGISTRY")},
			&cli.StringFlag{Name: "manifest-tag", Required: true, Sources: cli.EnvVars("MANIFEST_TAG")},
			&cli.StringFlag{Name: "manifest-file", Value: "final-manifest.txt"},
			outputFlag(), envFileFlag(),
		},
		Action: func(ctx context.Context, command *cli.Command) error {
			published, err := (ManifestService{}).Copy(ctx, &CopyManifestInput{
				ManifestTag: command.String("manifest-tag"), TargetRegistry: command.String("target-registry"),
				ImageName: command.String("image-name"), SourceTag: command.String("source-tag"),
				ManifestFile: command.String("manifest-file"),
			})
			if err != nil {
				return err
			}
			if err := appendWorkflowOutputs(
				command.String("github-output"),
				repositoryops.WorkflowValue{Name: "digest", Value: published.Digest},
				repositoryops.WorkflowValue{Name: "final-tag", Value: published.FinalTag},
				repositoryops.WorkflowValue{Name: "final-repo", Value: published.FinalRepository},
			); err != nil {
				return err
			}
			return appendWorkflowOutputs(
				command.String("github-env"),
				repositoryops.WorkflowValue{Name: "DIGEST", Value: published.Digest},
				repositoryops.WorkflowValue{Name: "FINAL_TAG", Value: published.FinalTag},
				repositoryops.WorkflowValue{Name: "FINAL_REPO", Value: published.FinalRepository},
			)
		},
	}
}

func newResolveManifestCommand() *cli.Command {
	return &cli.Command{
		Name: "resolve-manifest",
		Flags: []cli.Flag{
			imageNameFlag(), sourceTagFlag(),
			&cli.StringFlag{Name: "target-registry", Required: true, Sources: cli.EnvVars("TARGET_REGISTRY")},
			outputFlag(),
		},
		Action: func(ctx context.Context, command *cli.Command) error {
			finalTag := command.String("target-registry") + "/" + command.String("image-name") + ":" + command.String("source-tag")
			resolved, err := (ManifestService{}).Resolve(ctx, finalTag)
			if err != nil {
				return err
			}
			return appendWorkflowOutputs(
				command.String("github-output"),
				repositoryops.WorkflowValue{Name: "digest", Value: resolved.Digest},
				repositoryops.WorkflowValue{Name: "final-tag", Value: resolved.FinalTag},
			)
		},
	}
}

func newCosignCommand() *cli.Command {
	return &cli.Command{Name: "cosign", Commands: []*cli.Command{newCosignSignCommand()}}
}

func newCosignSignCommand() *cli.Command {
	return &cli.Command{
		Name: "sign",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "final-repo", Required: true, Sources: cli.EnvVars("FINAL_REPO")},
			&cli.StringFlag{Name: "digest", Required: true, Sources: cli.EnvVars("DIGEST")},
			&cli.StringFlag{Name: "bundle", Value: "/tmp/cosign-bundle.json"},
			&cli.StringFlag{Name: "output", Value: "/tmp/cosign-out"}, outputFlag(),
		},
		Action: func(ctx context.Context, command *cli.Command) error {
			result, err := (ManifestService{Stdout: os.Stdout, Stderr: os.Stderr}).Sign(ctx, SignManifestInput{
				Reference:  command.String("final-repo") + "@" + command.String("digest"),
				BundlePath: command.String("bundle"), OutputPath: command.String("output"),
			})
			if err != nil {
				return err
			}
			if result.RekorURL == "" {
				return nil
			}
			return appendWorkflowOutputs(command.String("github-output"), repositoryops.WorkflowValue{Name: "rekor-url", Value: result.RekorURL})
		},
	}
}

func newUpdatePreflightCommand() *cli.Command {
	return &cli.Command{
		Name: "update-preflight",
		Flags: []cli.Flag{
			repositoryFlag(), imageNameFlag(), sourceTagFlag(),
			&cli.StringFlag{Name: "source-ref", Required: true, Sources: cli.EnvVars("SOURCE_REF")},
			&cli.StringFlag{Name: "post-report", Value: "post.json"},
			&cli.IntFlag{Name: "attempts", Value: 5},
			&cli.DurationFlag{Name: "retry-delay", Value: 2 * time.Second},
		},
		Action: func(ctx context.Context, command *cli.Command) error {
			upstreamDigest := ""
			if resolved, err := (ManifestService{}).Resolve(ctx, command.String("source-ref")); err == nil {
				upstreamDigest = resolved.Digest
			}
			vulnerabilities := 0
			if summary, err := ReadTrivyReport(command.String("post-report")); err == nil {
				vulnerabilities = summary.Count
			}
			result, err := (PreflightService{}).Update(ctx, &PreflightRequest{
				Repository: command.String("repository"), ImageName: command.String("image-name"),
				SourceTag: command.String("source-tag"), UpstreamDigest: upstreamDigest,
				PatchedVulnerabilities: vulnerabilities, MaxAttempts: command.Int("attempts"),
				RetryDelay: command.Duration("retry-delay"),
			})
			if err != nil {
				return err
			}
			if !result.Updated {
				_, err = fmt.Fprintf(os.Stdout, "::warning::Preflight manifest update failed after %d attempts\n", command.Int("attempts"))
				return err
			}
			return nil
		},
	}
}
