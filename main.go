package main

import (
	"context"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"

	"github.com/verity-org/verity/cmd"
)

func main() {
	root := &cli.Command{
		Name:  "verity",
		Usage: "Self-maintaining registry of security-patched container images",
		Commands: []*cli.Command{
			cmd.ScanCommand,
			cmd.CatalogCommand,
			cmd.DiscoverCommand,
			cmd.IntegerCommand,
			cmd.PreflightCommand,
			cmd.ChartGenCommand,
			cmd.PatchCommand,
		},
		Version: "2.0.0",
	}

	if err := root.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
