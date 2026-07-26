package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/urfave/cli/v3"
)

type prIntegerBatchKind string

const (
	prIntegerBatchSmoke prIntegerBatchKind = "smoke"
	prIntegerBatchBuild prIntegerBatchKind = "build"
	prVerityFlagName                       = "verity"
)

type prIntegerBatchRequest struct {
	Kind                prIntegerBatchKind
	Entries             []prIntegerEntry
	Architecture        string
	PackageArchitecture string
	RepoRoot            string
	RunnerTemp          string
	SecurityDir         string
	ReportsDir          string
	VerityPath          string
}

type prNativePackageCheck struct {
	Kind, Architecture, RepoRoot string
}

type prSealedSecretsCheck struct {
	Image, Version, FullVersion, TempDir string
}

type prIntegerNativeChecks interface {
	TestPackage(context.Context, prNativePackageCheck) error
	VerifySealedSecretsImage(context.Context, prSealedSecretsCheck) error
}

type prHTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

type prIntegerDependencies struct {
	Commands   prIntegerCommandRunner
	Native     prIntegerNativeChecks
	HTTPClient prHTTPClient
	Wait       func(context.Context, time.Duration) error
	Stdout     io.Writer
	Stderr     io.Writer
}

func newCIPrIntegerBatchCommand() *cli.Command {
	return &cli.Command{
		Name:  "integer-batch",
		Usage: "Build and verify one native-architecture Integer PR batch",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "kind", Required: true},
			&cli.StringFlag{Name: "entries", Required: true},
			&cli.StringFlag{Name: "arch", Required: true},
			&cli.StringFlag{Name: "package-arch", Required: true},
			&cli.StringFlag{Name: "repo-root", Value: "."},
			&cli.StringFlag{Name: "runner-temp", Value: os.TempDir()},
			&cli.StringFlag{Name: "security-dir", Value: "integer-security"},
			&cli.StringFlag{Name: "reports-dir", Value: "trivy-reports"},
			&cli.StringFlag{Name: prVerityFlagName, Value: "./verity"},
		},
		Action: runCIPrIntegerBatch,
	}
}

func runCIPrIntegerBatch(ctx context.Context, command *cli.Command) error {
	request, err := newPRIntegerBatchRequest(command)
	if err != nil {
		return err
	}
	deps := &prIntegerDependencies{
		Commands: execPRIntegerCommandRunner{}, Native: repositoryPRIntegerChecks{},
		HTTPClient: &http.Client{Timeout: 5 * time.Second}, Wait: waitPRInteger,
		Stdout: command.Writer, Stderr: command.ErrWriter,
	}
	return executePRIntegerBatch(ctx, deps, &request)
}

func newPRIntegerBatchRequest(command *cli.Command) (prIntegerBatchRequest, error) {
	kind := prIntegerBatchKind(command.String("kind"))
	if kind != prIntegerBatchSmoke && kind != prIntegerBatchBuild {
		return prIntegerBatchRequest{}, fmt.Errorf("%w: invalid Integer batch kind %q", errPRCommandFailed, kind)
	}
	arch := command.String("arch")
	packageArch := command.String("package-arch")
	if (arch != "amd64" || packageArch != "x86_64") && (arch != "arm64" || packageArch != "aarch64") {
		return prIntegerBatchRequest{}, fmt.Errorf("%w: mismatched Integer architectures %q and %q", errPRCommandFailed, arch, packageArch)
	}
	var entries []prIntegerEntry
	decoder := json.NewDecoder(strings.NewReader(command.String("entries")))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&entries); err != nil || len(entries) == 0 {
		return prIntegerBatchRequest{}, fmt.Errorf("%w: Integer entries must be a non-empty JSON array", errPRCommandFailed)
	}
	if err := ensurePRJSONEOF(decoder); err != nil {
		return prIntegerBatchRequest{}, fmt.Errorf("%w: invalid Integer entries JSON", errPRCommandFailed)
	}
	for _, entry := range entries {
		if err := validatePRIntegerEntry(entry); err != nil {
			return prIntegerBatchRequest{}, err
		}
	}
	return prIntegerBatchRequest{
		Kind: kind, Entries: entries, Architecture: arch, PackageArchitecture: packageArch,
		RepoRoot: filepath.Clean(command.String("repo-root")), RunnerTemp: filepath.Clean(command.String("runner-temp")),
		SecurityDir: filepath.Clean(command.String("security-dir")), ReportsDir: filepath.Clean(command.String("reports-dir")),
		VerityPath: command.String(prVerityFlagName),
	}, nil
}

func executePRIntegerBatch(ctx context.Context, deps *prIntegerDependencies, request *prIntegerBatchRequest) error {
	normalizePRIntegerDependencies(deps)
	for _, dir := range []string{request.SecurityDir, request.ReportsDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create Integer PR output directory %q: %w", dir, err)
		}
	}
	for _, entry := range request.Entries {
		if err := executePRIntegerEntry(ctx, deps, request, entry); err != nil {
			return err
		}
	}
	return nil
}

func normalizePRIntegerDependencies(deps *prIntegerDependencies) {
	if deps.Commands == nil {
		deps.Commands = execPRIntegerCommandRunner{}
	}
	if deps.Native == nil {
		deps.Native = repositoryPRIntegerChecks{}
	}
	if deps.HTTPClient == nil {
		deps.HTTPClient = &http.Client{Timeout: 5 * time.Second}
	}
	if deps.Wait == nil {
		deps.Wait = waitPRInteger
	}
	if deps.Stdout == nil {
		deps.Stdout = io.Discard
	}
	if deps.Stderr == nil {
		deps.Stderr = io.Discard
	}
}

func waitPRInteger(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func validatePRIntegerEntry(entry prIntegerEntry) error {
	if strings.TrimSpace(entry.Image) == "" || strings.TrimSpace(entry.Version) == "" || strings.TrimSpace(entry.Type) == "" {
		return fmt.Errorf("%w: incomplete Integer batch entry", errPRCommandFailed)
	}
	safeImage := strings.NewReplacer("/", "-", ":", "-").Replace(entry.Image)
	if !prMarkerComponentPattern.MatchString(safeImage) || !prMarkerComponentPattern.MatchString(entry.Version) || !prMarkerComponentPattern.MatchString(entry.Type) {
		return fmt.Errorf("%w: unsafe Integer batch entry %s:%s-%s", errPRCommandFailed, entry.Image, entry.Version, entry.Type)
	}
	return nil
}
