package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/verity-org/verity/internal/discovery"
	"github.com/verity-org/verity/internal/integer/apkindex"
	intdiscovery "github.com/verity-org/verity/internal/integer/discovery"
)

// NightlyCommand owns scheduled scan/dispatch decisions. It deliberately keeps
// orchestration policy in Go so workflows only install tools and invoke Verity.
var NightlyCommand = &cli.Command{
	Name:  "nightly",
	Usage: "Plan and dispatch nightly remediation from current vulnerability scans",
	Commands: []*cli.Command{
		nightlyPlanCmd,
		nightlyDispatchCmd,
		nightlyCopaOrchestratorCmd,
	},
}

var nightlyPlanCmd = &cli.Command{
	Name:  "plan",
	Usage: "Scan published Verity images and emit only dirty remediation targets",
	Flags: []cli.Flag{
		&cli.StringFlag{Name: "family", Usage: "Image family to plan: copa or integer", Required: true},
		&cli.StringFlag{Name: "config", Usage: "Path to copa-config.yaml", Value: "copa-config.yaml"},
		&cli.StringFlag{Name: "charts-file", Usage: "Path to Chart.yaml", Value: "Chart.yaml"},
		&cli.StringFlag{Name: "verity-config", Usage: "Path to verity.yaml", Value: "verity.yaml"},
		&cli.StringFlag{Name: "integer-config", Usage: "Path to integer.yaml", Value: "integer.yaml"},
		&cli.StringFlag{Name: "images-dir", Usage: "Path to images/", Value: "images"},
		&cli.StringFlag{Name: "target-registry", Usage: "Target registry override"},
		&cli.StringFlag{Name: "apkindex-url", Usage: "Wolfi APKINDEX URL", Value: apkindex.DefaultAPKINDEXURL},
		&cli.StringFlag{Name: "cache-dir", Usage: "APKINDEX cache dir"},
		&cli.StringFlag{Name: "gen-dir", Usage: "Generated apko config directory"},
		&cli.StringFlag{Name: "only", Usage: "Comma-separated image names to consider"},
		&cli.IntFlag{Name: "parallel", Usage: "Number of concurrent target scans", Value: 6},
		&cli.BoolFlag{Name: "force", Usage: "Bypass scan skip and emit every considered target"},
		&cli.StringFlag{Name: "output", Usage: "Write dispatch matrix JSON to this file"},
		&cli.StringFlag{Name: "github-output", Usage: "Append count/images outputs to this GitHub output file"},
	},
	Action: func(ctx context.Context, cmd *cli.Command) error {
		family := cmd.String("family")
		force := cmd.Bool("force")
		parallel := max(cmd.Int("parallel"), 1)

		var data []byte
		var count int
		var integerShards []integerMatrixShard
		var err error
		switch family {
		case nightlyFamilyCopa:
			var items []discovery.DiscoveredImage
			items, err = nightlyPlanCopa(ctx, cmd, force, parallel)
			if err != nil {
				return err
			}
			if items == nil {
				items = []discovery.DiscoveredImage{}
			}
			count = len(items)
			data, err = json.Marshal(items)
		case nightlyFamilyInteger:
			var items []intdiscovery.DiscoveredImage
			items, err = nightlyPlanInteger(ctx, cmd, force, parallel)
			if err != nil {
				return err
			}
			if items == nil {
				items = []intdiscovery.DiscoveredImage{}
			}
			count = len(items)
			data, err = json.Marshal(items)
			if err == nil {
				integerShards, err = shardIntegerImages(items)
			}
		default:
			return fmt.Errorf("%w: %q; want %q or %q", errUnsupportedNightlyFamily, family, nightlyFamilyCopa, nightlyFamilyInteger)
		}
		if err != nil {
			return err
		}

		if out := cmd.String("output"); out != "" {
			if err := os.WriteFile(out, data, 0o644); err != nil {
				return fmt.Errorf("writing %s: %w", out, err)
			}
		}
		if ghOut := cmd.String("github-output"); ghOut != "" {
			if err := appendGitHubMatrixOutput(ghOut, count, data); err != nil {
				return err
			}
			if family == nightlyFamilyInteger {
				if err := appendGitHubShardOutput(ghOut, integerShards); err != nil {
					return err
				}
			}
		}
		fmt.Fprintln(os.Stdout, string(data))
		return nil
	},
}

var nightlyDispatchCmd = &cli.Command{
	Name:  "dispatch",
	Usage: "Dispatch a nightly remediation matrix through the GitHub Actions API",
	Flags: []cli.Flag{
		&cli.StringFlag{Name: "family", Usage: "Image family to dispatch: copa or integer", Required: true},
		&cli.StringFlag{Name: "input", Usage: "Dispatch matrix JSON file", Required: true},
		&cli.StringFlag{Name: "repo", Usage: "GitHub repository owner/name", Required: true},
		&cli.StringFlag{Name: "ref", Usage: "Git ref for workflow dispatch", Required: true},
		&cli.StringFlag{Name: "workflow", Usage: "Workflow file name; defaults from --family"},
		&cli.StringFlag{Name: "batch-id", Usage: "Correlation identifier passed to dispatched workflows"},
		&cli.IntFlag{Name: "retries", Usage: "Dispatch retries per item", Value: 5},
		&cli.DurationFlag{Name: "throttle", Usage: "Delay between successful dispatches", Value: 2 * time.Second},
	},
	Action: func(ctx context.Context, cmd *cli.Command) error {
		workflow := cmd.String("workflow")
		if workflow == "" {
			switch cmd.String("family") {
			case nightlyFamilyCopa:
				workflow = "patch-image.yaml"
			case nightlyFamilyInteger:
				workflow = "integer-build-image.yaml"
			default:
				return fmt.Errorf("%w: %q", errUnsupportedNightlyFamily, cmd.String("family"))
			}
		}
		token := os.Getenv("GH_TOKEN")
		if token == "" {
			token = os.Getenv("GITHUB_TOKEN")
		}
		if token == "" {
			return errMissingGitHubToken
		}

		inputs, err := nightlyDispatchInputs(cmd.String("family"), cmd.String("input"), cmd.String("batch-id"))
		if err != nil {
			return err
		}
		for index, input := range inputs {
			if err := dispatchWorkflow(ctx, token, cmd.String("repo"), workflow, cmd.String("ref"), input, cmd.Int("retries")); err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "Dispatched %d/%d to %s\n", index+1, len(inputs), workflow)
			if index+1 < len(inputs) && cmd.Duration("throttle") > 0 {
				time.Sleep(cmd.Duration("throttle"))
			}
		}
		fmt.Fprintf(os.Stderr, "✓ Dispatched %d %s remediation run(s)\n", len(inputs), cmd.String("family"))
		return nil
	},
}
