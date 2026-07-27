package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/verity-org/verity/internal/ci/sitepublication"
)

var errInvalidSitePublicationArguments = errors.New("invalid site publication arguments")

var ciSitePublicationCommand = &cli.Command{
	Name:  "site-publication",
	Usage: "Plan, assemble, sign, validate, and restore coherent Build Site artifacts",
	Commands: []*cli.Command{
		ciSitePublicationPlanCommand,
		ciSitePublicationAssembleCommand,
		ciSitePublicationSignerPlanCommand,
		ciSitePublicationSignerExecuteCommand,
		ciSitePublicationSignCommand,
		ciSitePublicationFinalizeCommand,
		ciSitePublicationResolvePreviousCommand,
		ciSitePublicationRestoreCommand,
	},
}

var _ = registerCISitePublicationCommand()

func registerCISitePublicationCommand() bool {
	CICommand.Commands = append(CICommand.Commands, ciSitePublicationCommand)
	return true
}

func requireSitePublicationArguments(command *cli.Command, count int) error {
	if command.Args().Len() != count {
		return fmt.Errorf("%w: %s expects %d positional arguments, received %d", errInvalidSitePublicationArguments, command.FullName(), count, command.Args().Len())
	}
	return nil
}

func readSitePublicationPlan(path string) (sitepublication.PublicationPlan, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return sitepublication.PublicationPlan{}, fmt.Errorf("read site publication plan %q: %w", path, err)
	}
	plan, err := sitepublication.ParsePlanCanonical(data)
	if err != nil {
		return sitepublication.PublicationPlan{}, fmt.Errorf("parse site publication plan %q: %w", path, err)
	}
	return plan, nil
}

func commandWriter(command *cli.Command) io.Writer {
	if command.Root().Writer == nil {
		return os.Stdout
	}
	return command.Root().Writer
}

func commandErrorWriter(command *cli.Command) io.Writer {
	if command.Root().ErrWriter == nil {
		return os.Stderr
	}
	return command.Root().ErrWriter
}

func writeMachineRecord(command *cli.Command, data []byte) error {
	if path := command.String("record-output"); path != "" {
		return writeAtomicSitePublicationRecord(path, data)
	}
	if _, err := commandWriter(command).Write(data); err != nil {
		return fmt.Errorf("write machine output: %w", err)
	}
	return nil
}

func writeAtomicSitePublicationRecord(path string, data []byte) (retErr error) {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("%w: record output path is empty", errInvalidSitePublicationArguments)
	}
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return fmt.Errorf("create site publication record directory %q: %w", directory, err)
	}
	temporary, err := os.CreateTemp(directory, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temporary site publication record for %q: %w", path, err)
	}
	temporaryPath := temporary.Name()
	defer func() {
		if removeErr := os.Remove(temporaryPath); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			retErr = errors.Join(retErr, fmt.Errorf("remove temporary site publication record %q: %w", temporaryPath, removeErr))
		}
	}()
	if _, err := temporary.Write(data); err != nil {
		return errors.Join(fmt.Errorf("write site publication record %q: %w", path, err), temporary.Close())
	}
	if err := temporary.Sync(); err != nil {
		return errors.Join(fmt.Errorf("sync site publication record %q: %w", path, err), temporary.Close())
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close site publication record %q: %w", path, err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace site publication record %q: %w", path, err)
	}
	return nil
}

type sitePublicationOutputField struct {
	name  string
	value string
}

func sitePublicationGitHubOutputPath(command *cli.Command) string {
	if path := command.String("github-output"); path != "" {
		return path
	}
	return os.Getenv("GITHUB_OUTPUT")
}

func appendSitePublicationGitHubOutputs(path string, fields ...sitePublicationOutputField) (retErr error) {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("%w: GitHub output path is empty", errInvalidSitePublicationArguments)
	}
	for _, field := range fields {
		if field.name == "" || strings.ContainsAny(field.name, "=\r\n") {
			return fmt.Errorf("%w: invalid GitHub output name %q", errInvalidSitePublicationArguments, field.name)
		}
		if field.value == "" || strings.ContainsAny(field.value, "\r\n") {
			return fmt.Errorf("%w: invalid GitHub output value for %q", errInvalidSitePublicationArguments, field.name)
		}
	}
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return fmt.Errorf("create GitHub output directory %q: %w", directory, err)
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open GitHub output %q: %w", path, err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			retErr = errors.Join(retErr, fmt.Errorf("close GitHub output %q: %w", path, closeErr))
		}
	}()
	for _, field := range fields {
		if _, err := fmt.Fprintf(file, "%s=%s\n", field.name, field.value); err != nil {
			return fmt.Errorf("write GitHub output %q: %w", path, err)
		}
	}
	return nil
}

func appendCISitePublicationPlanOutputs(path string, plan *sitepublication.PublicationPlan) error {
	return appendSitePublicationGitHubOutputs(
		path,
		sitePublicationOutputField{name: "plan_digest", value: string(plan.PlanDigest)},
		sitePublicationOutputField{name: "manifest_digest", value: string(plan.ManifestDigest)},
		sitePublicationOutputField{name: "mode", value: string(plan.Mode)},
	)
}

func appendCISitePublicationSignerOutputs(path string, result sitepublication.SignerResult) error {
	return appendSitePublicationGitHubOutputs(
		path,
		sitePublicationOutputField{name: "output_digest", value: string(result.OutputDigest)},
	)
}

func appendCISitePublicationFinalizeOutputs(path string, plan *sitepublication.FinalPlan) error {
	return appendSitePublicationGitHubOutputs(
		path,
		sitePublicationOutputField{name: "artifact_digest", value: string(plan.ArtifactDigest)},
		sitePublicationOutputField{name: "manifest_digest", value: string(plan.ManifestDigest)},
		sitePublicationOutputField{name: "deploy_eligible", value: strconv.FormatBool(plan.DeployEligible)},
	)
}

func parseOverlays(values []string) ([]sitepublication.Overlay, error) {
	overlays := make([]sitepublication.Overlay, 0, len(values))
	for _, value := range values {
		name, source, found := strings.Cut(value, "=")
		if !found || strings.TrimSpace(name) == "" || strings.TrimSpace(source) == "" {
			return nil, fmt.Errorf("%w: overlay must have NAME=SOURCE form", errInvalidSitePublicationArguments)
		}
		overlays = append(overlays, sitepublication.Overlay{Name: name, SourceDir: source})
	}
	return overlays, nil
}
