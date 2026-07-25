package patchimage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/verity-org/verity/internal/ci/repositoryops"
)

func newMergePlatformMetricsCommand() *cli.Command {
	return &cli.Command{
		Name: "merge-platform-metrics",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "directory", Required: true, Sources: cli.EnvVars("PLATFORM_METRICS_DIR")},
			outputFlag(),
		},
		Action: func(_ context.Context, command *cli.Command) error {
			platforms := ReadPlatformSet(command.String("directory"))
			return appendWorkflowOutputs(
				command.String("github-output"),
				repositoryops.WorkflowValue{Name: "amd64", Value: string(platforms.AMD64)},
				repositoryops.WorkflowValue{Name: "arm64", Value: string(platforms.ARM64)},
			)
		},
	}
}

func newBuildSuccessMetricsCommand() *cli.Command {
	outcomes := finalizeOutcomeFlags()
	flags := make([]cli.Flag, 0, 20+len(outcomes))
	flags = append(
		flags,
		repositoryFlag(), imageNameFlag(), sourceTagFlag(), outputFlag(),
		&cli.StringFlag{Name: "safe-name", Required: true, Sources: cli.EnvVars("SAFE_NAME")},
		&cli.StringFlag{Name: "run-id", Required: true, Sources: cli.EnvVars("GITHUB_RUN_ID")},
		&cli.StringFlag{Name: "run-attempt", Required: true, Sources: cli.EnvVars("GITHUB_RUN_ATTEMPT")},
		&cli.StringFlag{Name: "source-sha", Required: true, Sources: cli.EnvVars("SOURCE_SHA")},
		&cli.StringFlag{Name: "target-ref", Sources: cli.EnvVars("TARGET_REF")},
		&cli.StringFlag{Name: "manifest-digest", Sources: cli.EnvVars("MANIFEST_DIGEST")},
		&cli.StringFlag{Name: "vuln-before", Sources: cli.EnvVars("VULN_BEFORE")},
		&cli.StringFlag{Name: "vuln-after", Sources: cli.EnvVars("VULN_AFTER")},
		&cli.StringFlag{Name: "pre-report", Value: "pre.json"},
		&cli.StringFlag{Name: "post-report", Value: "post.json"},
		&cli.StringFlag{Name: "amd64", Value: "null", Sources: cli.EnvVars("AMD64")},
		&cli.StringFlag{Name: "arm64", Value: "null", Sources: cli.EnvVars("ARM64")},
		&cli.StringFlag{Name: "rekor-url", Sources: cli.EnvVars("REKOR_URL")},
		&cli.StringFlag{Name: "attestation-id", Sources: cli.EnvVars("ATTESTATION_ID")},
		&cli.StringFlag{Name: "sbom", Value: "sbom.json"},
		&cli.StringFlag{Name: "attestation-bundle-path", Sources: cli.EnvVars("ATTESTATION_BUNDLE_PATH")},
	)
	for _, outcome := range outcomes {
		flags = append(flags, &cli.StringFlag{Name: outcome.name, Sources: cli.EnvVars(outcome.environment)})
	}
	return &cli.Command{
		Name: "build-success-metrics", Flags: flags,
		Action: func(ctx context.Context, command *cli.Command) error {
			runID, err := parseRequiredInt64("run id", command.String("run-id"))
			if err != nil {
				return err
			}
			runAttempt, err := parseRequiredInt64("run attempt", command.String("run-attempt"))
			if err != nil {
				return err
			}
			startedAt, err := (WorkflowStartService{}).Fetch(ctx, command.String("repository"), command.String("run-id"))
			if err != nil {
				return err
			}
			before := reportOrFallback(command.String("pre-report"), parseCount(command.String("vuln-before")))
			after := reportOrFallback(command.String("post-report"), parseCount(command.String("vuln-after")))
			sbomDigest, digestErr := FileSHA256(command.String("sbom"))
			if digestErr != nil && !errors.Is(digestErr, os.ErrNotExist) {
				return digestErr
			}
			document := BuildSuccessMetrics(&SuccessMetricsInput{
				Run: RunMetrics{ID: runID, Attempt: runAttempt, StartedAt: startedAt, EndedAt: commandClock().Now()},
				Image: ImageMetrics{
					Name: command.String("image-name"), SourceTag: command.String("source-tag"),
					TargetRef: command.String("target-ref"), ManifestDigest: command.String("manifest-digest"),
				},
				Before: before, After: after, Outcomes: outcomeValues(command),
				Platforms: PlatformSet{AMD64: parsePlatformJSON(command.String("amd64")), ARM64: parsePlatformJSON(command.String("arm64"))},
				SupplyChain: SupplyChainInput{
					RekorURL: command.String("rekor-url"), AttestationID: command.String("attestation-id"),
					SBOMDigest: sbomDigest, AttestationBundlePath: command.String("attestation-bundle-path"),
				},
			})
			filename := metricsFilename(command.String("safe-name"), command.String("source-tag"))
			if err := WriteMetricsDocument(filename, &document); err != nil {
				return err
			}
			if err := printFile(filename); err != nil {
				return err
			}
			return writeMetricsOutputs(command, filename)
		},
	}
}

func newBuildFailureMetricsCommand() *cli.Command {
	return &cli.Command{
		Name: "build-failure-metrics",
		Flags: []cli.Flag{
			imageNameFlag(), sourceTagFlag(), outputFlag(),
			&cli.StringFlag{Name: "safe-name", Required: true, Sources: cli.EnvVars("SAFE_NAME")},
			&cli.StringFlag{Name: "run-id", Required: true, Sources: cli.EnvVars("GITHUB_RUN_ID")},
			&cli.StringFlag{Name: "run-attempt", Required: true, Sources: cli.EnvVars("GITHUB_RUN_ATTEMPT")},
			&cli.StringFlag{Name: "source-sha", Required: true, Sources: cli.EnvVars("SOURCE_SHA")},
			&cli.StringFlag{Name: "started-at", Required: true, Sources: cli.EnvVars("STARTED_AT")},
			&cli.StringFlag{Name: "platform-directory", Required: true, Sources: cli.EnvVars("PLATFORM_METRICS_DIR")},
		},
		Action: func(_ context.Context, command *cli.Command) error {
			runID, err := parseRequiredInt64("run id", command.String("run-id"))
			if err != nil {
				return err
			}
			runAttempt, err := parseRequiredInt64("run attempt", command.String("run-attempt"))
			if err != nil {
				return err
			}
			document := BuildFailureMetrics(&FailureMetricsInput{
				Run:       RunMetrics{ID: runID, Attempt: runAttempt, StartedAt: command.String("started-at"), EndedAt: commandClock().Now()},
				ImageName: command.String("image-name"), SourceTag: command.String("source-tag"),
				Platforms: ReadPlatformSet(command.String("platform-directory")),
			})
			filename := metricsFilename(command.String("safe-name"), command.String("source-tag"))
			if err := WriteMetricsDocument(filename, &document); err != nil {
				return err
			}
			if err := printFile(filename); err != nil {
				return err
			}
			return writeMetricsOutputs(command, filename)
		},
	}
}

type outcomeFlag struct {
	name        string
	environment string
}

func finalizeOutcomeFlags() []outcomeFlag {
	return []outcomeFlag{
		{name: "harden-outcome", environment: "HARDEN_OUTCOME"},
		{name: "checkout-outcome", environment: "CHECKOUT_OUTCOME"},
		{name: "mise-outcome", environment: "MISE_OUTCOME"},
		{name: "login-outcome", environment: "LOGIN_OUTCOME"},
		{name: "scan-artifacts-outcome", environment: "SCAN_ARTIFACTS_OUTCOME"},
		{name: "manifest-outcome", environment: "MANIFEST_OUTCOME"},
		{name: "postscan-outcome", environment: "POSTSCAN_OUTCOME"},
		{name: "compare-outcome", environment: "COMPARE_OUTCOME"},
		{name: "push-outcome", environment: "PUSH_OUTCOME"},
		{name: "cosign-outcome", environment: "COSIGN_OUTCOME"},
		{name: "sbom-outcome", environment: "SBOM_OUTCOME"},
		{name: "attest-outcome", environment: "ATTEST_OUTCOME"},
		{name: "otel-cache-outcome", environment: "OTEL_CACHE_OUTCOME"},
		{name: "otel-install-outcome", environment: "OTEL_INSTALL_OUTCOME"},
		{name: "otel-emit-outcome", environment: "OTEL_EMIT_OUTCOME"},
		{name: "push-reports-outcome", environment: "PUSH_REPORTS_OUTCOME"},
		{name: "preflight-outcome", environment: "PREFLIGHT_OUTCOME"},
		{name: "upload-pr-scan-outcome", environment: "UPLOAD_PR_SCAN_OUTCOME"},
		{name: "pl-download-outcome", environment: "PL_DOWNLOAD_OUTCOME"},
		{name: "pl-merge-outcome", environment: "PL_MERGE_OUTCOME"},
	}
}

func outcomeValues(command *cli.Command) []string {
	flags := finalizeOutcomeFlags()
	values := make([]string, 0, len(flags))
	for _, flag := range flags {
		values = append(values, command.String(flag.name))
	}
	return values
}

func reportOrFallback(path string, count int) TrivySummary {
	summary, err := ReadTrivyReport(path)
	if err == nil {
		return summary
	}
	return TrivySummary{Count: count, BySeverity: baseSeverityCounts()}
}

func writeMetricsOutputs(command *cli.Command, filename string) error {
	safeName := command.String("safe-name")
	sourceTag := command.String("source-tag")
	artifactName := strings.TrimSuffix(metricsFilename(safeName, sourceTag), ".json")
	if _, err := fmt.Fprintf(os.Stdout, "metrics artifact=%s-%s\n", safeName, sourceTag); err != nil {
		return fmt.Errorf("print metrics result: %w", err)
	}
	return appendWorkflowOutputs(
		command.String("github-output"),
		repositoryops.WorkflowValue{Name: "filename", Value: filename},
		repositoryops.WorkflowValue{Name: "artifact-name", Value: artifactName},
		repositoryops.WorkflowValue{Name: "run-id", Value: command.String("run-id")},
		repositoryops.WorkflowValue{Name: "run-attempt", Value: command.String("run-attempt")},
		repositoryops.WorkflowValue{Name: "source-sha", Value: command.String("source-sha")},
	)
}
