package cmd

import "github.com/urfave/cli/v3"

// yamlExt is the file extension used for image / bespoke yaml files. Shared
// across the integer subcommands so a future rename ripples cleanly.
const yamlExt = ".yaml"

// latestSentinel is the special version string that means "resolve to the
// highest version available in the APKINDEX at run time". Shared across
// build, validate, and sync subcommands so the sentinel value lives in
// exactly one place. Also matches the package-private const in
// internal/integer/discovery (kept in sync by the build path).
const latestSentinel = "latest"

// IntegerCommand is the top-level "integer" subcommand group for managing
// Wolfi-based OCI images built from source.
var IntegerCommand = &cli.Command{
	Name:  "integer",
	Usage: "Build and manage Wolfi-based OCI images from source",
	Commands: []*cli.Command{
		integerDiscoverCmd,
		integerValidateCmd,
		integerBuildCmd,
		integerMelangeCmd,
		integerSyncCmd,
		integerCatalogCmd,
	},
}
