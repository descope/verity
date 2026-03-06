package cmd

import "github.com/urfave/cli/v2"

// IntegerCommand is the top-level "integer" subcommand group for managing
// Wolfi-based OCI images built from source.
var IntegerCommand = &cli.Command{
	Name:  "integer",
	Usage: "Build and manage Wolfi-based OCI images from source",
	Subcommands: []*cli.Command{
		integerDiscoverCmd,
		integerValidateCmd,
		integerBuildCmd,
		integerSyncCmd,
		integerCatalogCmd,
	},
}
