package main

import (
	"context"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"

	"github.com/verity-org/verity/cmd"
	"github.com/verity-org/verity/internal/buildmetadata"
)

func main() {
	metadata, err := buildmetadata.CurrentValidated()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error: invalid build metadata")
		os.Exit(1)
	}
	root := &cli.Command{
		Name:  "verity",
		Usage: "Self-maintaining registry of security-patched container images",
		Commands: []*cli.Command{
			cmd.ScanCommand,
			cmd.CatalogCommand,
			cmd.DiscoverCommand,
			cmd.IntegerCommand,
			cmd.CICommand,
			cmd.NightlyCommand,
			cmd.PreflightCommand,
			cmd.ChartGenCommand,
			cmd.PatchCommand,
			cmd.DoctorCommand,
			cmd.VersionCommand,
		},
		Version: metadata.Version,
	}

	if err := root.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
