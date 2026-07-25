package patchimage

import (
	"context"
	"path/filepath"
	"runtime"
	"strconv"

	"github.com/urfave/cli/v3"

	"github.com/verity-org/verity/internal/ci/repositoryops"
)

func newPlatformRequestedCommand() *cli.Command {
	return &cli.Command{
		Name: "platform-requested",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "platforms", Required: true, Sources: cli.EnvVars("PLATFORMS")},
			&cli.StringFlag{Name: "platform", Required: true, Sources: cli.EnvVars("PLATFORM")},
			outputFlag(),
		},
		Action: func(_ context.Context, command *cli.Command) error {
			return appendWorkflowOutputs(command.String("github-output"), repositoryops.WorkflowValue{
				Name: "enabled", Value: strconv.FormatBool(PlatformRequested(command.String("platforms"), command.String("platform"))),
			})
		},
	}
}

func newWritePlatformMetricsCommand() *cli.Command {
	return &cli.Command{
		Name: "write-platform-metrics",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "arch", Required: true, Sources: cli.EnvVars("ARCH")},
			&cli.StringFlag{Name: "duration", Sources: cli.EnvVars("DURATION")},
			&cli.StringFlag{Name: "exit-code", Sources: cli.EnvVars("EXIT_CODE")},
			&cli.StringFlag{Name: "digest", Sources: cli.EnvVars("DIGEST")},
			&cli.StringFlag{Name: "runner-temp", Required: true, Sources: cli.EnvVars("RUNNER_TEMP")},
			outputFlag(),
		},
		Action: func(_ context.Context, command *cli.Command) error {
			path := platformMetadataPath(command.String("runner-temp"), command.String("arch"))
			metrics := NewPlatformMetrics(PlatformMetricsInput{
				Arch: command.String("arch"), Duration: command.String("duration"),
				ExitCode: command.String("exit-code"), Digest: command.String("digest"),
			})
			if err := WritePlatformMetrics(path, metrics); err != nil {
				return err
			}
			if err := printFile(path); err != nil {
				return err
			}
			return appendWorkflowOutputs(command.String("github-output"), repositoryops.WorkflowValue{Name: "path", Value: path})
		},
	}
}

func newInstallOtelCommand() *cli.Command {
	return &cli.Command{
		Name: "install-otel",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "version", Required: true, Sources: cli.EnvVars("OTEL_CLI_VERSION")},
			&cli.StringFlag{Name: "sha256-linux-amd64", Required: true, Sources: cli.EnvVars("OTEL_CLI_SHA256_LINUX_AMD64")},
			&cli.StringFlag{Name: "sha256-linux-arm64", Required: true, Sources: cli.EnvVars("OTEL_CLI_SHA256_LINUX_ARM64")},
			&cli.StringFlag{Name: "home", Required: true, Sources: cli.EnvVars("HOME")},
			&cli.StringFlag{Name: "github-path", Required: true, Sources: cli.EnvVars("GITHUB_PATH")},
		},
		Action: func(ctx context.Context, command *cli.Command) error {
			expected := command.String("sha256-linux-amd64")
			if runtime.GOARCH == "arm64" {
				expected = command.String("sha256-linux-arm64")
			}
			return (OtelInstaller{}).Install(ctx, &OtelInstallInput{
				Version: command.String("version"), GOOS: runtime.GOOS, GOARCH: runtime.GOARCH,
				ExpectedSHA256: expected, HomeDir: command.String("home"), GitHubPath: command.String("github-path"),
			})
		},
	}
}

func newEmitPlatformSpanCommand() *cli.Command {
	return &cli.Command{
		Name: "emit-platform-span",
		Flags: []cli.Flag{
			imageNameFlag(), sourceTagFlag(),
			&cli.StringFlag{Name: "platform", Required: true, Sources: cli.EnvVars("PLATFORM")},
			&cli.StringFlag{Name: "cve-before", Sources: cli.EnvVars("CVE_BEFORE")},
			&cli.StringFlag{Name: "copa-exit", Sources: cli.EnvVars("COPA_EXIT")},
			&cli.StringFlag{Name: "copa-duration", Sources: cli.EnvVars("COPA_DURATION")},
			&cli.StringFlag{Name: "staging-digest", Sources: cli.EnvVars("STAGING_DIGEST")},
			&cli.StringFlag{Name: "report", Value: "pre.json"},
			&cli.StringFlag{Name: "otel-path"},
			&cli.StringFlag{Name: "home", Required: true, Sources: cli.EnvVars("HOME")},
		},
		Action: func(ctx context.Context, command *cli.Command) error {
			otelPath := command.String("otel-path")
			if otelPath == "" {
				otelPath = filepath.Join(command.String("home"), ".local", "bin", "otel-cli")
			}
			return EmitPlatformSpan(ctx, nil, &PlatformSpanInput{
				OtelPath: otelPath, ReportPath: command.String("report"), ImageName: command.String("image-name"),
				Platform: command.String("platform"), SourceTag: command.String("source-tag"),
				CVEBefore: command.String("cve-before"), CopaExit: command.String("copa-exit"),
				CopaDuration: command.String("copa-duration"), StagingDigest: command.String("staging-digest"),
			})
		},
	}
}
