package cmd

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/urfave/cli/v3"
)

type prTestScope struct {
	Integer bool
	Copa    bool
}

var (
	prIntegerPath = regexp.MustCompile(`^(images/|integer\.yaml$|packages/(bespoke|pipelines|overrides)/|packages/upstream\.lock\.json$|internal/(ci|integer)/|cmd/(ci|integer|nightly|discover|scan).*\.go$|\.github/scripts/test-sealed-secrets-(package|image)\.sh$|\.github/workflows/(pr-test|integer-.*)\.yaml$)`)
	prCopaPath    = regexp.MustCompile(`^(copa-config\.yaml$|internal/patch/|cmd/(ci|nightly|patch|discover|scan).*\.go$|\.github/scripts/(patch-image|scan-before|verify-patched|read-catalog-entry)\.sh$|\.github/workflows/(pr-test|patch-image)\.yaml$|\.github/actions/setup-binaries/)`)
)

func newCIPrScopeCommand() *cli.Command {
	return &cli.Command{
		Name:  "scope",
		Usage: "Classify changed paths into Integer and Copa PR suites",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "base-sha"},
			&cli.StringFlag{Name: "head-sha"},
			&cli.StringFlag{Name: "changed-files"},
			&cli.StringFlag{Name: "repo-root", Value: "."},
			&cli.StringFlag{Name: "github-output", Required: true},
		},
		Action: runCIPrScope,
	}
}

func runCIPrScope(ctx context.Context, command *cli.Command) error {
	paths, err := loadPRChangedPaths(ctx, prChangedPathRequest{
		BaseSHA:     command.String("base-sha"),
		HeadSHA:     command.String("head-sha"),
		ChangedFile: command.String("changed-files"),
		RepoRoot:    command.String("repo-root"),
	})
	if err != nil {
		return err
	}
	scope := classifyPRTestPaths(paths)
	if err := appendPRGitHubValues(command.String("github-output"), [][2]string{
		{integerCommandName, strconv.FormatBool(scope.Integer)},
		{"copa", strconv.FormatBool(scope.Copa)},
	}); err != nil {
		return err
	}
	_, err = fmt.Fprintf(command.Writer, "PR scope: integer=%t copa=%t changed-paths=%d\n", scope.Integer, scope.Copa, len(paths))
	if err != nil {
		return fmt.Errorf("write PR scope summary: %w", err)
	}
	return nil
}

func classifyPRTestPaths(paths []string) prTestScope {
	var scope prTestScope
	for _, path := range paths {
		path = strings.TrimSpace(path)
		scope.Integer = scope.Integer || prIntegerPath.MatchString(path)
		scope.Copa = scope.Copa || prCopaPath.MatchString(path)
	}
	return scope
}

type prChangedPathRequest struct {
	BaseSHA     string
	HeadSHA     string
	ChangedFile string
	RepoRoot    string
}

func loadPRChangedPaths(ctx context.Context, request prChangedPathRequest) ([]string, error) {
	if request.ChangedFile != "" {
		data, err := os.ReadFile(request.ChangedFile)
		if err != nil {
			return nil, fmt.Errorf("read changed files %q: %w", request.ChangedFile, err)
		}
		lines := strings.Split(string(data), "\n")
		paths := make([]string, 0, len(lines))
		for _, path := range lines {
			if path = strings.TrimSpace(path); path != "" {
				paths = append(paths, path)
			}
		}
		return paths, nil
	}
	if strings.TrimSpace(request.BaseSHA) == "" || strings.TrimSpace(request.HeadSHA) == "" {
		return nil, fmt.Errorf("%w: base-sha and head-sha are required", errPRCommandFailed)
	}
	result, err := requirePRCommand(ctx, &prCommandRequest{
		Name: "git",
		Args: []string{"diff", "--name-only", "-z", request.BaseSHA + "..." + request.HeadSHA},
		Dir:  request.RepoRoot,
	})
	if err != nil {
		return nil, fmt.Errorf("discover changed PR paths: %w", err)
	}
	raw := strings.Split(string(result.Stdout), "\x00")
	paths := make([]string, 0, len(raw))
	for _, path := range raw {
		if path != "" {
			paths = append(paths, path)
		}
	}
	return paths, nil
}
