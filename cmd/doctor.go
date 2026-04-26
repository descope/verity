package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"

	"github.com/verity-org/verity/internal/doctor"
)

// DoctorCommand lints Verity's configuration cross-references — currently
// the orphan-replacements check, with more checks to follow. Useful as a
// pre-merge gate (e.g. local dev or CI) to catch silent config drift before
// it produces broken wrapper charts.
var DoctorCommand = &cli.Command{
	Name:  "doctor",
	Usage: "Lint Verity configuration files for known silent-failure patterns",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "charts-file",
			Usage: "Helm Chart.yaml whose dependencies: provides chart specs",
			Value: "Chart.yaml",
		},
		&cli.StringFlag{
			Name:  "verity-config",
			Usage: "Path to verity.yaml (replacements + overrides)",
			Value: "verity.yaml",
		},
		&cli.BoolFlag{
			Name:  "json",
			Usage: "Emit issues as JSON instead of human text (useful for CI parsing)",
		},
		&cli.BoolFlag{
			Name:  "fail-on-warning",
			Usage: "Exit non-zero when any warning is reported (default: only errors fail)",
		},
	},
	Action: func(_ context.Context, c *cli.Command) error {
		issues, err := doctor.Run(doctor.Config{
			ChartsFile:   c.String("charts-file"),
			VerityConfig: c.String("verity-config"),
		})
		if err != nil {
			return fmt.Errorf("verity doctor: %w", err)
		}

		if c.Bool("json") {
			out, jerr := json.MarshalIndent(issues, "", "  ")
			if jerr != nil {
				return fmt.Errorf("marshal doctor output: %w", jerr)
			}
			fmt.Fprintln(os.Stdout, string(out))
		} else {
			fmt.Fprint(os.Stdout, doctor.FormatText(issues))
		}

		if doctor.HasErrors(issues) {
			return cli.Exit("verity doctor: errors reported", 1)
		}
		if c.Bool("fail-on-warning") && len(issues) > 0 {
			return cli.Exit("verity doctor: warnings reported and --fail-on-warning is set", 1)
		}
		return nil
	},
}
