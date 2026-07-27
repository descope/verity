package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/urfave/cli/v3"

	"github.com/verity-org/verity/internal/ci"
)

func newCIPrPlanCopaCommand() *cli.Command {
	return &cli.Command{
		Name:  "plan-copa",
		Usage: "Discover changed Copa images and emit the PR matrix",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "base-sha", Required: true},
			&cli.StringFlag{Name: "head-sha", Required: true},
			&cli.StringFlag{Name: "repo-root", Value: "."},
			&cli.StringFlag{Name: "temp-dir", Value: os.TempDir()},
			&cli.StringFlag{Name: "config", Value: "copa-config.yaml"},
			&cli.StringFlag{Name: "target-registry"},
			&cli.StringFlag{Name: "github-output", Required: true},
		},
		Action: runCIPrPlanCopa,
	}
}

func runCIPrPlanCopa(ctx context.Context, command *cli.Command) error {
	paths, err := loadPRChangedPaths(ctx, prChangedPathRequest{
		BaseSHA: command.String("base-sha"), HeadSHA: command.String("head-sha"), RepoRoot: command.String("repo-root"),
	})
	if err != nil {
		return err
	}
	root, err := os.MkdirTemp(command.String("temp-dir"), "verity-pr-copa-")
	if err != nil {
		return fmt.Errorf("create Copa plan directory: %w", err)
	}
	defer os.RemoveAll(root)
	baseConfig := filepath.Join(root, "base-copa-config.yaml")
	data, ok, err := gitShowPRFile(ctx, prGitShowRequest{
		RepoRoot: command.String("repo-root"), Revision: command.String("base-sha"), Path: command.String("config"),
	})
	if err != nil {
		return err
	}
	if !ok {
		data = []byte("images: []\n")
	}
	if err := writePRFile(baseConfig, data); err != nil {
		return err
	}
	plan, err := ci.PlanCopaPR(&ci.CopaPROptions{
		ChangedFiles: paths, BaseConfigPath: baseConfig, HeadConfigPath: command.String("config"), TargetRegistry: command.String("target-registry"),
	})
	if err != nil {
		return fmt.Errorf("plan Copa PR tests: %w", err)
	}
	matrix, err := marshalPRJSON(plan.Matrix)
	if err != nil {
		return err
	}
	if err := appendPRGitHubValues(command.String("github-output"), [][2]string{
		{"matrix", matrix},
		{"has-changes", strconv.FormatBool(plan.HasChanges)},
	}); err != nil {
		return err
	}
	_, err = fmt.Fprintf(command.Writer, "Copa change matrix: %d images to test\n", len(plan.Matrix.Include))
	if err != nil {
		return fmt.Errorf("write Copa plan summary: %w", err)
	}
	return nil
}
