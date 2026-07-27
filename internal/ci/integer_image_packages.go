package ci

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/verity-org/verity/internal/integer/apkindex"
	intconfig "github.com/verity-org/verity/internal/integer/config"
	"github.com/verity-org/verity/internal/integer/melange"
)

type IntegerPackageTestOptions struct {
	Architecture IntegerArchitecture
	Workspace    string
	Timeout      time.Duration
	Runner       IntegerImageRunner
}

type integerPackageTestRun struct {
	architecture IntegerArchitecture
	workspace    string
	timeout      time.Duration
	runner       IntegerImageRunner
	buildFiles   []string
	versions     map[string]string
	usePipelines bool
}

func TestIntegerPackages(ctx context.Context, options *IntegerPackageTestOptions) error {
	run, err := newIntegerPackageTestRun(options)
	if err != nil {
		return err
	}
	for _, buildFile := range run.buildFiles {
		if err := run.testPackage(ctx, buildFile); err != nil {
			return err
		}
	}
	return nil
}

func newIntegerPackageTestRun(options *IntegerPackageTestOptions) (*integerPackageTestRun, error) {
	if options == nil || !validIntegerArchitecture(options.Architecture) || options.Workspace == "" || options.Timeout <= 0 {
		return nil, fmt.Errorf("%w: native package test options", ErrIntegerBatchPlan)
	}
	buildFiles, err := filepath.Glob(filepath.Join(options.Workspace, "melange-work", "specs", "*", "build.yaml"))
	if err != nil {
		return nil, fmt.Errorf("find staged package specs: %w", err)
	}
	slices.Sort(buildFiles)
	if len(buildFiles) == 0 {
		return nil, fmt.Errorf("%w: no staged package specs", ErrIntegerBatchPlan)
	}
	indexPath := filepath.Join(options.Workspace, "packages", "repo", string(options.Architecture), "APKINDEX.tar.gz")
	indexData, err := os.ReadFile(indexPath)
	if err != nil {
		return nil, fmt.Errorf("read local package index: %w", err)
	}
	localVersions, err := integerPackageVersions(indexData)
	if err != nil {
		return nil, err
	}
	runner := options.Runner
	if runner == nil {
		runner = execIntegerImageRunner{}
	}
	pipelines := filepath.Join(options.Workspace, "melange-work", "pipelines")
	_, pipelineErr := os.Stat(pipelines)
	usePipelines := pipelineErr == nil
	if pipelineErr != nil && !os.IsNotExist(pipelineErr) {
		return nil, fmt.Errorf("inspect staged package pipelines: %w", pipelineErr)
	}
	return &integerPackageTestRun{
		architecture: options.Architecture,
		workspace:    options.Workspace,
		timeout:      options.Timeout,
		runner:       runner,
		buildFiles:   buildFiles,
		versions:     localVersions,
		usePipelines: usePipelines,
	}, nil
}

func integerPackageVersions(indexData []byte) (map[string]string, error) {
	localPackages, err := apkindex.ParseArchive(bytes.NewReader(indexData))
	if err != nil {
		return nil, fmt.Errorf("parse local package index: %w", err)
	}
	versions := make(map[string]string, len(localPackages))
	for _, localPackage := range localPackages {
		if !integerPackageNamePattern.MatchString(localPackage.Name) || !intconfig.ValidMelangeVersion(localPackage.Version) {
			return nil, fmt.Errorf("%w: invalid local package identity %q=%q", ErrIntegerBatchPlan, localPackage.Name, localPackage.Version)
		}
		if _, exists := versions[localPackage.Name]; exists {
			return nil, fmt.Errorf("%w: duplicate local package %s", ErrIntegerBatchPlan, localPackage.Name)
		}
		versions[localPackage.Name] = localPackage.Version
	}
	return versions, nil
}

func (run *integerPackageTestRun) testPackage(ctx context.Context, buildFile string) error {
	packageName, err := integerPackageNameFromSpec(buildFile)
	if err != nil {
		return err
	}
	buildData, err := os.ReadFile(buildFile)
	if err != nil {
		return fmt.Errorf("read staged package test spec: %w", err)
	}
	testData, err := pinIntegerPackageTestSpec(buildData, run.versions)
	if err != nil {
		return fmt.Errorf("pin package tests for %s: %w", packageName, err)
	}
	testFile := filepath.Join(filepath.Dir(buildFile), "test.yaml")
	if err := os.WriteFile(testFile, testData, 0o600); err != nil {
		return fmt.Errorf("write pinned package test spec: %w", err)
	}
	relativeBuild, err := filepath.Rel(run.workspace, testFile)
	if err != nil {
		return fmt.Errorf("make package spec relative: %w", err)
	}
	args := []string{
		"test", "--arch", string(run.architecture),
		"--repository-append", filepath.Join(run.workspace, "packages", "repo"),
		"--repository-append", melange.RepositoryURL,
		"--keyring-append", filepath.Join(run.workspace, "packages", "repo", "melange-"+string(run.architecture)+".rsa.pub"),
		"--keyring-append", melange.KeyringURL,
		"--test-package-append", "busybox",
		"--runner", "docker",
	}
	if run.usePipelines {
		args = append(args, "--pipeline-dirs", "melange-work/pipelines")
	}
	args = append(args, filepath.ToSlash(relativeBuild), packageName)
	commandCtx, cancel := context.WithTimeout(ctx, run.timeout)
	_, runErr := run.runner.Run(commandCtx, IntegerImageCommand{Name: "melange", Args: args, Dir: run.workspace})
	cancel()
	if runErr != nil {
		return fmt.Errorf("test package %s on %s: %w", packageName, run.architecture, runErr)
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
