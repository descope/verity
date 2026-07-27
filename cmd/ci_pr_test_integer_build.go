package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

func runPRMelangeBuild(
	ctx context.Context,
	deps *prIntegerDependencies,
	request *prIntegerBatchRequest,
	entry prIntegerEntry,
	staged bool,
) error {
	args := []string{
		integerCommandName, "melange", "build", "--image", entry.Image, "--version", entry.Version,
		"--type", entry.Type, "--arch", request.PackageArchitecture,
	}
	if staged {
		args = append(args, "--staged")
	}
	commandContext, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	_, err := runPRIntegerCommand(commandContext, deps, &prCommandRequest{
		Name: request.VerityPath, Args: args, Dir: request.RepoRoot, TermGrace: time.Minute,
	})
	return err
}

func runPRIntegerPackageChecks(
	ctx context.Context,
	deps *prIntegerDependencies,
	request *prIntegerBatchRequest,
	entry prIntegerEntry,
) error {
	if request.Kind == prIntegerBatchBuild && entry.Image != "pushgateway" {
		return nil
	}
	switch entry.Image {
	case "pushgateway":
		return runPRMelangePackageTest(ctx, deps, request, "pushgateway.yaml", "prometheus-pushgateway", 5*time.Minute)
	case "weaviate":
		if request.Kind == prIntegerBatchSmoke {
			return runPRMelangePackageTest(ctx, deps, request, "weaviate.yaml", "weaviate", 5*time.Minute)
		}
	case "rclone", "sealed-secrets", "step-ca":
		if request.Kind == prIntegerBatchSmoke {
			return deps.Native.TestPackage(ctx, prNativePackageCheck{
				Kind: entry.Image, Architecture: request.PackageArchitecture, RepoRoot: request.RepoRoot,
			})
		}
	}
	return nil
}

func runPRMelangePackageTest(
	ctx context.Context,
	deps *prIntegerDependencies,
	request *prIntegerBatchRequest,
	buildDir, packageName string,
	timeout time.Duration,
) error {
	commandContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	_, err := runPRIntegerCommand(commandContext, deps, &prCommandRequest{
		Name: "melange", Dir: request.RepoRoot, TermGrace: time.Minute,
		Args: []string{
			"test", "--arch", request.PackageArchitecture,
			"--repository-append", filepath.Join(request.RepoRoot, "packages", "repo"),
			"--repository-append", "https://packages.wolfi.dev/os",
			"--keyring-append", filepath.Join(
				request.RepoRoot,
				"packages",
				"repo",
				"melange-"+request.PackageArchitecture+".rsa.pub",
			),
			"--keyring-append", "https://packages.wolfi.dev/os/wolfi-signing.rsa.pub",
			"--pipeline-dirs", "melange-work/pipelines", "--runner", "docker",
			filepath.ToSlash(filepath.Join("melange-work", "specs", buildDir, "build.yaml")), packageName,
		},
	})
	if err != nil {
		return fmt.Errorf("test %s package for %s: %w", packageName, request.PackageArchitecture, err)
	}
	return nil
}

func runPRIntegerBuild(
	ctx context.Context,
	deps *prIntegerDependencies,
	request *prIntegerBatchRequest,
	entry prIntegerEntry,
	tarPath string,
) error {
	_, err := runPRIntegerCommand(ctx, deps, &prCommandRequest{
		Name: request.VerityPath, Dir: request.RepoRoot,
		Args: []string{
			integerCommandName, "build", "--image", entry.Image, "--version", entry.Version,
			"--type", entry.Type, "--output", tarPath, "--arch", request.Architecture,
			"--fail-on-severity", prIntegerSeverities,
		},
	})
	if err != nil {
		return fmt.Errorf("build strict Integer image: %w", err)
	}
	return nil
}

func runPRIntegerTrivy(
	ctx context.Context,
	deps *prIntegerDependencies,
	root, tarPath, reportPath string,
) (int, error) {
	if _, err := runPRIntegerCommand(ctx, deps, &prCommandRequest{
		Name: "trivy", Dir: root,
		Args: []string{
			"image", "--exit-code", "1", "--severity", prIntegerSeverities,
			"--vuln-type", "os,library", "--format", "json", "--output", reportPath,
			"--input", tarPath,
		},
	}); err != nil {
		return 0, fmt.Errorf("strict Trivy scan: %w", err)
	}
	total, err := readPRTrivyTotal(reportPath)
	if err != nil {
		return 0, err
	}
	if total != 0 {
		return total, fmt.Errorf("%w: strict Trivy report contains %d vulnerabilities", errPRCommandFailed, total)
	}
	return total, nil
}

func readPRTrivyTotal(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("read Trivy report: %w", err)
	}
	var report struct {
		Results []struct {
			Vulnerabilities []json.RawMessage `json:"Vulnerabilities"`
		} `json:"Results"`
	}
	if err := json.Unmarshal(data, &report); err != nil {
		return 0, fmt.Errorf("parse Trivy report: %w", err)
	}
	total := 0
	for _, result := range report.Results {
		total += len(result.Vulnerabilities)
	}
	return total, nil
}

type prPackageMetadata struct {
	Version     string
	FullVersion string
}

func readPRPackageMetadata(root, buildDir string) (prPackageMetadata, error) {
	path := filepath.Join(root, "melange-work", "specs", buildDir, "build.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return prPackageMetadata{}, fmt.Errorf("read package metadata %q: %w", path, err)
	}
	var spec struct {
		Package struct {
			Version string `yaml:"version"`
			Epoch   int    `yaml:"epoch"`
		} `yaml:"package"`
	}
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return prPackageMetadata{}, fmt.Errorf("parse package metadata %q: %w", path, err)
	}
	version := strings.TrimSpace(spec.Package.Version)
	if !prMarkerComponentPattern.MatchString(version) || spec.Package.Epoch < 0 {
		return prPackageMetadata{}, fmt.Errorf("%w: invalid package metadata in %q", errPRCommandFailed, path)
	}
	return prPackageMetadata{Version: version, FullVersion: fmt.Sprintf("%s-r%d", version, spec.Package.Epoch)}, nil
}

func runPRIntegerCommand(
	ctx context.Context,
	deps *prIntegerDependencies,
	request *prCommandRequest,
) (prCommandResult, error) {
	if request.Stdout == nil {
		request.Stdout = deps.Stdout
	}
	if request.Stderr == nil {
		request.Stderr = deps.Stderr
	}
	return deps.Commands.Run(ctx, request)
}
