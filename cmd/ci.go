package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/verity-org/verity/internal/ci"
	"github.com/verity-org/verity/internal/integer/apkindex"
)

var CICommand = &cli.Command{
	Name:  "ci",
	Usage: "CI planning helpers",
	Commands: []*cli.Command{
		ciPlanCommand,
	},
}

var ciPlanCommand = &cli.Command{
	Name:  "plan",
	Usage: "Emit a typed CI matrix plan as JSON",
	Flags: []cli.Flag{
		&cli.StringFlag{Name: "kind", Required: true, Usage: "Plan kind: integer-pr, copa-pr, chart"},
		&cli.StringFlag{Name: "changed-files", Usage: "Path to newline-delimited changed files"},
		&cli.StringFlag{Name: "event-name", Usage: "GitHub event name", Value: "pull_request"},
		&cli.StringFlag{Name: "input-chart", Usage: "Single chart requested by workflow_dispatch"},
		&cli.StringFlag{Name: "integer-config", Usage: "Path to integer.yaml", Value: "integer.yaml"},
		&cli.StringFlag{Name: "images-dir", Usage: "Path to images/", Value: "images"},
		&cli.StringFlag{Name: "apkindex-url", Usage: "Wolfi APKINDEX URL; empty disables online discovery", Value: apkindex.DefaultAPKINDEXURL},
		&cli.StringFlag{Name: "cache-dir", Usage: "APKINDEX cache dir"},
		&cli.StringFlag{Name: "gen-dir", Usage: "Generated apko config directory"},
		&cli.StringFlag{Name: "base-copa-config", Usage: "Base copa-config.yaml for PR diffing"},
		&cli.StringFlag{Name: "copa-config", Usage: "Head copa-config.yaml", Value: "copa-config.yaml"},
		&cli.StringFlag{Name: "target-registry", Usage: "Target registry override"},
		&cli.StringFlag{Name: "charts-file", Usage: "Head Chart.yaml", Value: "Chart.yaml"},
		&cli.StringFlag{Name: "base-charts-file", Usage: "Base Chart.yaml for PR diffing"},
		&cli.StringFlag{Name: "verity-config", Usage: "Path to verity.yaml", Value: "verity.yaml"},
		&cli.StringFlag{Name: "values-dir", Usage: "Path to chart integration values", Value: "test/chart-integration/values"},
	},
	Action: runCIPlan,
}

func runCIPlan(_ context.Context, cmd *cli.Command) error {
	changed, err := readChangedFiles(cmd.String("changed-files"))
	if err != nil {
		return err
	}

	var plan ci.Plan
	switch cmd.String("kind") {
	case "integer-pr":
		plan, err = ci.PlanIntegerPR(ci.IntegerPROptions{
			ChangedFiles: changed,
			ConfigPath:   cmd.String("integer-config"),
			ImagesDir:    cmd.String("images-dir"),
			APKIndexURL:  cmd.String("apkindex-url"),
			CacheDir:     cmd.String("cache-dir"),
			GenDir:       cmd.String("gen-dir"),
		})
	case "copa-pr":
		plan, err = ci.PlanCopaPR(ci.CopaPROptions{
			ChangedFiles:   changed,
			BaseConfigPath: cmd.String("base-copa-config"),
			HeadConfigPath: cmd.String("copa-config"),
			TargetRegistry: cmd.String("target-registry"),
			ChartsFile:     cmd.String("charts-file"),
			VerityConfig:   cmd.String("verity-config"),
		})
	case "chart":
		plan, err = ci.PlanCharts(ci.ChartOptions{
			EventName:      cmd.String("event-name"),
			InputChart:     cmd.String("input-chart"),
			ChangedFiles:   changed,
			ChartsFile:     cmd.String("charts-file"),
			BaseChartsFile: cmd.String("base-charts-file"),
			VerityConfig:   cmd.String("verity-config"),
			ValuesDir:      cmd.String("values-dir"),
		})
	default:
		return fmt.Errorf("unknown plan kind %q", cmd.String("kind"))
	}
	if err != nil {
		return err
	}

	out, err := ci.Marshal(plan)
	if err != nil {
		return fmt.Errorf("marshal ci plan: %w", err)
	}
	fmt.Fprintln(os.Stdout, string(out))
	return nil
}

func readChangedFiles(path string) ([]string, error) {
	if strings.TrimSpace(path) == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read changed files %q: %w", path, err)
	}
	lines := strings.Split(string(data), "\n")
	files := make([]string, 0, len(lines))
	for _, line := range lines {
		if line = strings.TrimSpace(line); line != "" {
			files = append(files, line)
		}
	}
	return files, nil
}
