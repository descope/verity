package cmd

import "github.com/urfave/cli/v3"

// yamlExt is the file extension used for image / bespoke yaml files. Shared
// across the integer subcommands so a future rename ripples cleanly.
const yamlExt = ".yaml"

const integerCommandName = "integer"

// latestSentinel is the special version string that means "resolve to the
// highest version available in the APKINDEX at run time". Shared across
// build, validate, and sync subcommands so the sentinel value lives in
// exactly one place. Also matches the package-private const in
// internal/integer/discovery (kept in sync by the build path).
const latestSentinel = "latest"

// IntegerCommand is the top-level subcommand group for managing bespoke OCI images.
var IntegerCommand = &cli.Command{
	Name:  integerCommandName,
	Usage: "Build and manage bespoke OCI images from source",
	Commands: []*cli.Command{
		integerDiscoverCmd,
		integerValidateCmd,
		integerBuildCmd,
		integerMetadataCmd,
		integerMelangeCmd,
		integerSyncCmd,
		integerCatalogCmd,
	},
}
