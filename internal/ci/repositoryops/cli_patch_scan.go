package repositoryops

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/urfave/cli/v3"
)

func newPatchImageCLICommand(deps *cliDependencies) *cli.Command {
	return &cli.Command{
		Name: "patch-image",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "platform", Required: true}, &cli.StringFlag{Name: "source", Required: true},
			&cli.StringFlag{Name: "image-name", Required: true}, &cli.StringFlag{Name: "staging-registry", Required: true},
			&cli.StringFlag{Name: "go-vcs-url"}, &cli.StringFlag{Name: "report"}, &cli.StringFlag{Name: "github-output"},
		},
		Action: func(ctx context.Context, command *cli.Command) error {
			started := time.Now()
			request, err := NewPatchRequest(&PatchRequestInput{
				Platform: command.String("platform"), Source: command.String("source"), ImageName: command.String("image-name"),
				StagingRegistry: command.String("staging-registry"), GoVCSURL: command.String("go-vcs-url"),
				Report: command.String("report"),
			})
			if err != nil {
				return err
			}
			result, runErr := (PatchService{Patcher: deps.patcher, Commands: deps.commands}).Run(ctx, &request)
			values := []WorkflowValue{
				{Name: "exit-code", Value: strconv.Itoa(boolExitCode(runErr))},
				{Name: "duration-seconds", Value: strconv.FormatInt(int64(time.Since(started)/time.Second), 10)},
			}
			var summaryErr error
			if runErr == nil {
				values = append(values, WorkflowValue{Name: "staging-digest", Value: result.Digest})
				if _, err := fmt.Fprintf(deps.stdout, "Patched platform-specific image: %s\n", result.Destination); err != nil {
					summaryErr = fmt.Errorf("write patch summary: %w", err)
				}
			}
			outputErr := appendCLIWorkflowValues(command.String("github-output"), deps.getenv("GITHUB_OUTPUT"), values)
			return errors.Join(runErr, summaryErr, outputErr)
		},
	}
}

func newScanBeforeCLICommand(deps *cliDependencies) *cli.Command {
	return &cli.Command{
		Name: "scan-before",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "source", Required: true}, &cli.StringFlag{Name: "report", Value: "before.json"},
			&cli.StringFlag{Name: "github-env"},
		},
		Action: func(ctx context.Context, command *cli.Command) error {
			request, err := NewScanBeforeRequest(ScanBeforeInput{Source: command.String("source"), ReportPath: command.String("report")})
			if err != nil {
				return err
			}
			result, err := (ScanService{Commands: deps.commands}).Before(ctx, request)
			if err != nil {
				return err
			}
			values := countWorkflowValues("before", result.Counts)
			if result.SkipPatch {
				values = append(values, WorkflowValue{Name: "skip_patch", Value: "true"})
			}
			if _, err := fmt.Fprintf(
				deps.stdout,
				"BEFORE — total: %d, non-go: %d, go: %d\n",
				result.Counts.Total,
				result.Counts.NonGo,
				result.Counts.Go,
			); err != nil {
				return fmt.Errorf("write pre-patch scan summary: %w", err)
			}
			return appendCLIWorkflowValues(command.String("github-env"), deps.getenv("GITHUB_ENV"), values)
		},
	}
}

func newVerifyPatchedCLICommand(deps *cliDependencies) *cli.Command {
	return &cli.Command{
		Name: "verify-patched",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "image", Required: true}, &cli.StringFlag{Name: "image-label", Required: true},
			&cli.StringFlag{Name: "report", Value: "after.json"}, &cli.IntFlag{Name: "before-total", Required: true},
			&cli.IntFlag{Name: "before-go", Required: true}, &cli.IntFlag{Name: "before-non-go", Required: true},
		},
		Action: func(ctx context.Context, command *cli.Command) error {
			request, err := NewVerifyPatchedRequest(VerifyPatchedInput{
				Image: command.String("image"), ImageLabel: command.String("image-label"), ReportPath: command.String("report"),
				Before: VulnerabilityCounts{Total: command.Int("before-total"), Go: command.Int("before-go"), NonGo: command.Int("before-non-go")},
			})
			if err != nil {
				return err
			}
			result, verifyErr := (ScanService{Commands: deps.commands}).Verify(ctx, request)
			_, summaryErr := fmt.Fprintf(
				deps.stdout,
				"%s — BEFORE %d (%d non-Go, %d Go); AFTER %d (%d non-Go, %d Go)\n",
				command.String("image-label"),
				result.Before.Total,
				result.Before.NonGo,
				result.Before.Go,
				result.After.Total,
				result.After.NonGo,
				result.After.Go,
			)
			if summaryErr != nil {
				summaryErr = fmt.Errorf("write patched-image verification summary: %w", summaryErr)
			}
			return errors.Join(verifyErr, summaryErr)
		},
	}
}

func newCatalogEntryCLICommand(deps *cliDependencies) *cli.Command {
	return &cli.Command{
		Name: "catalog-entry",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "config", Value: "copa-config.yaml"}, &cli.StringFlag{Name: "image-name", Required: true},
			&cli.StringFlag{Name: "image-tag", Required: true}, &cli.StringFlag{Name: "github-output"},
		},
		Action: func(_ context.Context, command *cli.Command) error {
			request, err := NewCatalogRequest(CatalogRequestInput{ConfigPath: command.String("config"), ImageName: command.String("image-name"), ImageTag: command.String("image-tag")})
			if err != nil {
				return err
			}
			entry, err := ReadCatalogEntry(request)
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(deps.stdout, "Catalog: image=%s goVcsUrl=%s\n", entry.Source, entry.GoVCSURL); err != nil {
				return fmt.Errorf("write catalog summary: %w", err)
			}
			return appendCLIWorkflowValues(command.String("github-output"), deps.getenv("GITHUB_OUTPUT"), []WorkflowValue{
				{Name: "source", Value: entry.Source}, {Name: "go_vcs_url", Value: entry.GoVCSURL},
			})
		},
	}
}
