package ci

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/verity-org/verity/internal/integer/melange"
)

type IntegerPackageTestOptions struct {
	Architecture IntegerArchitecture
	Workspace    string
	Timeout      time.Duration
	Runner       IntegerImageRunner
}

func TestIntegerPackages(ctx context.Context, options *IntegerPackageTestOptions) error {
	if options == nil || !validIntegerArchitecture(options.Architecture) || options.Workspace == "" || options.Timeout <= 0 {
		return fmt.Errorf("%w: native package test options", ErrIntegerBatchPlan)
	}
	buildFiles, err := filepath.Glob(filepath.Join(options.Workspace, "melange-work", "specs", "*", "build.yaml"))
	if err != nil {
		return fmt.Errorf("find staged package specs: %w", err)
	}
	slices.Sort(buildFiles)
	if len(buildFiles) == 0 {
		return fmt.Errorf("%w: no staged package specs", ErrIntegerBatchPlan)
	}
	runner := options.Runner
	if runner == nil {
		runner = execIntegerImageRunner{}
	}
	pipelines := filepath.Join(options.Workspace, "melange-work", "pipelines")
	_, pipelineErr := os.Stat(pipelines)
	usePipelines := pipelineErr == nil
	if pipelineErr != nil && !os.IsNotExist(pipelineErr) {
		return fmt.Errorf("inspect staged package pipelines: %w", pipelineErr)
	}
	for _, buildFile := range buildFiles {
		packageName, err := integerPackageNameFromSpec(buildFile)
		if err != nil {
			return err
		}
		relativeBuild, err := filepath.Rel(options.Workspace, buildFile)
		if err != nil {
			return fmt.Errorf("make package spec relative: %w", err)
		}
		args := []string{
			"test", "--arch", string(options.Architecture),
			"--repository-append", filepath.Join(options.Workspace, "packages", "repo"),
			"--repository-append", melange.RepositoryURL,
			"--keyring-append", filepath.Join(options.Workspace, "melange-work", "melange.rsa.pub"),
			"--keyring-append", melange.KeyringURL,
			"--runner", "docker",
		}
		if usePipelines {
			args = append(args, "--pipeline-dirs", "melange-work/pipelines")
		}
		args = append(args, filepath.ToSlash(relativeBuild), packageName)
		commandCtx, cancel := context.WithTimeout(ctx, options.Timeout)
		_, runErr := runner.Run(commandCtx, IntegerImageCommand{Name: "melange", Args: args, Dir: options.Workspace})
		cancel()
		if runErr != nil {
			return fmt.Errorf("test package %s on %s: %w", packageName, options.Architecture, runErr)
		}
	}
	return nil
}

func integerPackageNameFromSpec(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read staged package spec: %w", err)
	}
	var spec struct {
		Package struct {
			Name string `yaml:"name"`
		} `yaml:"package"`
	}
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return "", fmt.Errorf("parse staged package spec: %w", err)
	}
	if !integerPackageNamePattern.MatchString(spec.Package.Name) {
		return "", fmt.Errorf("%w: staged package name", ErrIntegerBatchPlan)
	}
	return spec.Package.Name, nil
}
