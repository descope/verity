package patchimage

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/urfave/cli/v3"

	"github.com/verity-org/verity/internal/ci/repositoryops"
)

var ErrInvalidCommandInput = errors.New("invalid patch-image command input")

func NewCommand() *cli.Command {
	return &cli.Command{
		Name:  "patch-image",
		Usage: "Typed scan, patch, finalize, telemetry, and metrics workflow operations",
		Commands: []*cli.Command{
			newParseInputsCommand(),
			newTrivyDateCommand(),
			newScanSourceCommand(),
			newCheckExistingCommand(),
			newDownloadPreviousReportCommand(),
			newPlatformRequestedCommand(),
			newWritePlatformMetricsCommand(),
			newInstallOtelCommand(),
			newEmitPlatformSpanCommand(),
			newCreateManifestCommand(),
			newScanPostCommand(),
			newCompareReportsCommand(),
			newCraneCommand(),
			newResolveManifestCommand(),
			newCosignCommand(),
			newUpdatePreflightCommand(),
			newMergePlatformMetricsCommand(),
			newBuildSuccessMetricsCommand(),
			newWorkflowStartCommand(),
			newBuildFailureMetricsCommand(),
		},
	}
}

func outputFlag() cli.Flag {
	return &cli.StringFlag{Name: "github-output", Hidden: true, Sources: cli.EnvVars("GITHUB_OUTPUT")}
}

func envFileFlag() cli.Flag {
	return &cli.StringFlag{Name: "github-env", Hidden: true, Sources: cli.EnvVars("GITHUB_ENV")}
}

func repositoryFlag() cli.Flag {
	return &cli.StringFlag{Name: "repository", Sources: cli.EnvVars("GITHUB_REPOSITORY")}
}

func imageNameFlag() cli.Flag {
	return &cli.StringFlag{Name: "image-name", Sources: cli.EnvVars("IMAGE_NAME")}
}

func sourceTagFlag() cli.Flag {
	return &cli.StringFlag{Name: "source-tag", Sources: cli.EnvVars("SOURCE_TAG")}
}

func appendWorkflowOutputs(path string, values ...repositoryops.WorkflowValue) error {
	return repositoryops.AppendWorkflowValues(path, values)
}

func parseRequiredInt64(label, value string) (int64, error) {
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed < 1 {
		return 0, fmt.Errorf("%w: %s must be a positive integer", ErrInvalidCommandInput, label)
	}
	return parsed, nil
}

func parseCount(value string) int {
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return 0
	}
	return parsed
}

func parsePlatformJSON(value string) json.RawMessage {
	if !json.Valid([]byte(value)) {
		return json.RawMessage("null")
	}
	return json.RawMessage(value)
}

func printFile(path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read generated file %q: %w", path, err)
	}
	if _, err := os.Stdout.Write(content); err != nil {
		return fmt.Errorf("print generated file %q: %w", path, err)
	}
	return nil
}

func metricsFilename(safeName, sourceTag string) string {
	return fmt.Sprintf("metrics-%s-%s.json", safeName, sourceTag)
}

func platformMetadataPath(runnerTemp, arch string) string {
	return filepath.Join(runnerTemp, "platform-"+arch+".json")
}

func commandClock() Clock {
	return systemClock{}
}
