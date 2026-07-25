package cmd

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/urfave/cli/v3"
	"golang.org/x/tools/cover"

	repositoryops "github.com/verity-org/verity/internal/ci/repositoryops"
	"github.com/verity-org/verity/internal/ci/workflowpolicy"
)

var (
	errInvalidWorkflowArguments = errors.New("invalid workflow policy arguments")
	errInvalidCICommit          = errors.New("invalid CI commit SHA")
	errInvalidCoverageProfile   = errors.New("invalid Go coverage profile")
	errInvalidCoverageMinimum   = errors.New("invalid coverage minimum")
	errCoverageBelowMinimum     = errors.New("coverage below required minimum")
	ciCommitPattern             = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)
)

var CIWorkflowCommand = registerCIWorkflowCommand()

func registerCIWorkflowCommand() *cli.Command {
	command := newCIWorkflowCommand()
	CICommand.Commands = append(CICommand.Commands, command)
	return command
}

func newCIWorkflowCommand() *cli.Command {
	return &cli.Command{
		Name:  "workflow",
		Usage: "Validate typed GitHub Actions workflow policy",
		Commands: []*cli.Command{
			{
				Name:      "validate",
				Usage:     "Validate workflow ownership, identity, pinning, privilege, and zero-CVE policy",
				ArgsUsage: "WORKFLOWS_DIR",
				Action:    runCIWorkflowValidate,
			},
			{
				Name:  "scope",
				Usage: "Emit selective CI job outputs from typed changed-path rules",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "event-name", Required: true},
					&cli.StringFlag{Name: "base-sha"},
					&cli.StringFlag{Name: "head-sha"},
					&cli.StringFlag{Name: "repo-root", Value: "."},
					&cli.StringFlag{Name: "github-output"},
				},
				Action: runCIWorkflowScope,
			},
			{
				Name:  "coverage",
				Usage: "Enforce the Go statement coverage threshold",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "profile", Required: true},
					&cli.Float64Flag{Name: "minimum", Value: 80},
				},
				Action: runCIWorkflowCoverage,
			},
		},
	}
}

func runCIWorkflowValidate(_ context.Context, command *cli.Command) error {
	if command.Args().Len() != 1 {
		return fmt.Errorf(
			"%w: %s expects one workflow directory, received %d arguments",
			errInvalidWorkflowArguments,
			command.FullName(),
			command.Args().Len(),
		)
	}
	report, err := workflowpolicy.ValidateDirectory(command.Args().First())
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(command.Writer, "workflow policy validated: %d workflows\n", report.WorkflowCount); err != nil {
		return fmt.Errorf("write workflow validation result: %w", err)
	}
	return nil
}

type ciScopeRequest struct {
	eventName string
	baseSHA   string
	headSHA   string
	repoRoot  string
}

type ciPathScope struct {
	tests    bool
	goChecks bool
	static   bool
	node     bool
}

func runCIWorkflowScope(ctx context.Context, command *cli.Command) error {
	request := ciScopeRequest{
		eventName: strings.TrimSpace(command.String("event-name")),
		baseSHA:   strings.TrimSpace(command.String("base-sha")),
		headSHA:   strings.TrimSpace(command.String("head-sha")),
		repoRoot:  command.String("repo-root"),
	}
	scope := ciPathScope{tests: true, goChecks: true, static: true, node: true}
	if request.eventName == "pull_request" {
		paths, err := changedCIPaths(ctx, request)
		if err != nil {
			return err
		}
		scope = classifyCIPaths(paths)
	}
	outputPath := command.String("github-output")
	if outputPath == "" {
		outputPath = os.Getenv("GITHUB_OUTPUT")
	}
	return repositoryops.AppendWorkflowValues(outputPath, []repositoryops.WorkflowValue{
		{Name: "tests", Value: strconv.FormatBool(scope.tests)},
		{Name: "go", Value: strconv.FormatBool(scope.goChecks)},
		{Name: "static", Value: strconv.FormatBool(scope.static)},
		{Name: "node", Value: strconv.FormatBool(scope.node)},
	})
}

func changedCIPaths(ctx context.Context, request ciScopeRequest) ([]string, error) {
	if !ciCommitPattern.MatchString(request.baseSHA) {
		return nil, fmt.Errorf("%w: base %q", errInvalidCICommit, request.baseSHA)
	}
	if !ciCommitPattern.MatchString(request.headSHA) {
		return nil, fmt.Errorf("%w: head %q", errInvalidCICommit, request.headSHA)
	}
	git := exec.CommandContext(ctx, "git", "diff", "--name-only", "-z", request.baseSHA+"..."+request.headSHA, "--")
	git.Dir = request.repoRoot
	output, err := git.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("list changed CI paths: %w: %s", err, strings.TrimSpace(string(output)))
	}
	parts := strings.Split(string(output), "\x00")
	paths := make([]string, 0, len(parts))
	for _, path := range parts {
		if path != "" {
			paths = append(paths, path)
		}
	}
	return paths, nil
}

func classifyCIPaths(paths []string) ciPathScope {
	var scope ciPathScope
	for _, path := range paths {
		goModule := isGoModulePath(path)
		goSource := strings.HasSuffix(path, ".go")
		scope.tests = scope.tests || pathAffectsTests(path, goModule, goSource)
		scope.goChecks = scope.goChecks || pathAffectsGoChecks(path, goModule, goSource)
		scope.static = scope.static || pathAffectsStaticChecks(path)
		scope.node = scope.node || pathAffectsNodeChecks(path)
	}
	return scope
}

func isGoModulePath(path string) bool {
	return path == "go.mod" || path == "go.sum" ||
		strings.HasSuffix(path, "/go.mod") || strings.HasSuffix(path, "/go.sum")
}

func pathAffectsTests(path string, goModule, goSource bool) bool {
	return goModule || goSource || strings.HasPrefix(path, "cmd/") || strings.HasPrefix(path, "internal/") ||
		strings.HasPrefix(path, "scripts/") || path == ".github/workflows/ci.yaml" || path == "Makefile" || path == "mise.toml"
}

func pathAffectsGoChecks(path string, goModule, goSource bool) bool {
	return goModule || goSource || path == ".golangci.yml" || path == ".golangci.yaml"
}

func pathAffectsStaticChecks(path string) bool {
	return strings.HasPrefix(path, ".github/") || strings.HasPrefix(path, "scripts/") ||
		strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml") || strings.HasSuffix(path, ".md") ||
		path == "Makefile" || path == "mise.toml"
}

func pathAffectsNodeChecks(path string) bool {
	return strings.HasPrefix(path, "site/") || path == ".github/workflows/ci.yaml" || path == "mise.toml"
}

func runCIWorkflowCoverage(_ context.Context, command *cli.Command) error {
	minimum := command.Float64("minimum")
	if math.IsNaN(minimum) || math.IsInf(minimum, 0) || minimum < 0 || minimum > 100 {
		return fmt.Errorf("%w: %.1f", errInvalidCoverageMinimum, minimum)
	}
	percentage, err := coveragePercentage(command.String("profile"))
	if err != nil {
		return err
	}
	percentage = math.Round(percentage*10) / 10
	if _, err := fmt.Fprintf(command.Writer, "Coverage: %.1f%%\n", percentage); err != nil {
		return fmt.Errorf("write coverage result: %w", err)
	}
	if percentage < minimum {
		return fmt.Errorf(
			"coverage %.1f%% is below required %.1f%%: %w",
			percentage,
			minimum,
			errCoverageBelowMinimum,
		)
	}
	return nil
}

func coveragePercentage(path string) (float64, error) {
	profiles, err := cover.ParseProfiles(path)
	if err != nil {
		return 0, fmt.Errorf("%w %q: %w", errInvalidCoverageProfile, path, err)
	}
	coveredStatements := 0
	totalStatements := 0
	for _, profile := range profiles {
		for _, block := range profile.Blocks {
			totalStatements += block.NumStmt
			if block.Count > 0 {
				coveredStatements += block.NumStmt
			}
		}
	}
	if totalStatements == 0 {
		return 0, fmt.Errorf("%w %q: no statements", errInvalidCoverageProfile, path)
	}
	return 100 * float64(coveredStatements) / float64(totalStatements), nil
}
