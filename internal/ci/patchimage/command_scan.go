package patchimage

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/urfave/cli/v3"

	"github.com/verity-org/verity/internal/ci/repositoryops"
)

func newParseInputsCommand() *cli.Command {
	return &cli.Command{
		Name: "parse-inputs",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "source-ref", Required: true, Sources: cli.EnvVars("SOURCE_REF")},
			&cli.StringFlag{Name: "image-name", Required: true, Sources: cli.EnvVars("IMAGE_NAME")},
			&cli.StringFlag{Name: "target-registry", Required: true, Sources: cli.EnvVars("TARGET_REGISTRY")},
			outputFlag(),
		},
		Action: func(_ context.Context, command *cli.Command) error {
			parsed := ParseInputs(WorkflowInputs{
				SourceRef: command.String("source-ref"), ImageName: command.String("image-name"),
				TargetRegistry: command.String("target-registry"),
			})
			return appendWorkflowOutputs(
				command.String("github-output"),
				repositoryops.WorkflowValue{Name: "source-tag", Value: parsed.SourceTag},
				repositoryops.WorkflowValue{Name: "safe-name", Value: parsed.SafeName},
				repositoryops.WorkflowValue{Name: "staging-registry", Value: parsed.StagingRegistry},
			)
		},
	}
}

func newTrivyDateCommand() *cli.Command {
	return &cli.Command{
		Name:  "trivy-date",
		Flags: []cli.Flag{outputFlag()},
		Action: func(_ context.Context, command *cli.Command) error {
			return appendWorkflowOutputs(command.String("github-output"), repositoryops.WorkflowValue{
				Name: "date", Value: TrivyDateKey(commandClock().Now()),
			})
		},
	}
}

func newScanSourceCommand() *cli.Command {
	return newScanCommand("scan-source", "Source scan", "pre.json")
}

func newScanPostCommand() *cli.Command {
	return newScanCommand("scan-post", "Post-patch vulnerabilities", "post.json")
}

func newScanCommand(name, label, defaultReport string) *cli.Command {
	return &cli.Command{
		Name: name,
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "image", Required: true, Sources: cli.EnvVars("IMAGE")},
			&cli.StringFlag{Name: "report", Value: defaultReport},
			outputFlag(),
		},
		Action: func(ctx context.Context, command *cli.Command) error {
			summary, err := (ScanService{}).Scan(ctx, ScanRequest{Image: command.String("image"), ReportPath: command.String("report")})
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(os.Stdout, "%s: %d vulnerabilities\n", label, summary.Count); err != nil {
				return fmt.Errorf("print scan count: %w", err)
			}
			return appendWorkflowOutputs(command.String("github-output"), repositoryops.WorkflowValue{
				Name: "vuln-count", Value: strconv.Itoa(summary.Count),
			})
		},
	}
}

func newCheckExistingCommand() *cli.Command {
	return &cli.Command{
		Name: "check-existing",
		Flags: []cli.Flag{
			imageNameFlag(), sourceTagFlag(),
			&cli.StringFlag{Name: "target-registry", Required: true, Sources: cli.EnvVars("TARGET_REGISTRY")},
			&cli.StringFlag{Name: "report", Value: "patched-existing.json"},
			outputFlag(),
		},
		Action: func(ctx context.Context, command *cli.Command) error {
			reference := command.String("target-registry") + "/" + command.String("image-name") + ":" + command.String("source-tag")
			result, err := (ScanService{}).CheckExisting(ctx, ExistingImageRequest{Image: reference, ReportPath: command.String("report")})
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(os.Stdout, "Existing patched image vulnerabilities: %d\n", result.Count); err != nil {
				return fmt.Errorf("print existing scan count: %w", err)
			}
			return appendWorkflowOutputs(command.String("github-output"), repositoryops.WorkflowValue{
				Name: "needs-patch", Value: strconv.FormatBool(result.NeedsPatch),
			})
		},
	}
}

func newDownloadPreviousReportCommand() *cli.Command {
	return &cli.Command{
		Name: "download-previous-report",
		Flags: []cli.Flag{
			repositoryFlag(), imageNameFlag(), sourceTagFlag(),
			&cli.StringFlag{Name: "destination", Value: "prev-post.json"}, outputFlag(),
		},
		Action: func(ctx context.Context, command *cli.Command) error {
			result, err := (PreviousReportService{}).Download(ctx, PreviousReportRequest{
				Repository: command.String("repository"), ImageName: command.String("image-name"),
				SourceTag: command.String("source-tag"), Destination: command.String("destination"),
			})
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(os.Stdout, "Previous report exists=%t bytes=%d\n", result.Exists, result.Bytes); err != nil {
				return fmt.Errorf("print previous report result: %w", err)
			}
			return appendWorkflowOutputs(command.String("github-output"), repositoryops.WorkflowValue{
				Name: "exists", Value: strconv.FormatBool(result.Exists),
			})
		},
	}
}

func newWorkflowStartCommand() *cli.Command {
	return &cli.Command{
		Name: "workflow-start",
		Flags: []cli.Flag{
			repositoryFlag(),
			&cli.StringFlag{Name: "run-id", Required: true, Sources: cli.EnvVars("GITHUB_RUN_ID")},
			outputFlag(),
		},
		Action: func(ctx context.Context, command *cli.Command) error {
			startedAt, err := (WorkflowStartService{}).Fetch(ctx, command.String("repository"), command.String("run-id"))
			if err != nil {
				return err
			}
			return appendWorkflowOutputs(command.String("github-output"), repositoryops.WorkflowValue{Name: "value", Value: startedAt})
		},
	}
}
