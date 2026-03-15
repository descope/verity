package cmd

import "github.com/urfave/cli/v3"

// IntegerCommand is the top-level "integer" subcommand group for managing
// Wolfi-based OCI images built from source.
var IntegerCommand = &cli.Command{
	Name:  "integer",
	Usage: "Build and manage Wolfi-based OCI images from source",
	Commands: []*cli.Command{
		integerDiscoverCmd,
		integerValidateCmd,
		integerBuildCmd,
		integerSyncCmd,
		integerCatalogCmd,
	},
}
