package main

import (
	"fmt"
	"os"

	"github.com/urfave/cli/v2"

	"github.com/verity-org/verity/cmd"
)

func main() {
	app := &cli.App{
		Name:  "verity",
		Usage: "Self-maintaining registry of security-patched container images",
		Commands: []*cli.Command{
			cmd.PostProcessCommand,
			cmd.AssembleCommand,
			cmd.CatalogCommand,
		},
		Version: "2.0.0",
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
