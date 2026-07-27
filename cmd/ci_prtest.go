package cmd

import "github.com/urfave/cli/v3"

var CIPrTestCommand = registerCIPrTestCommand()

func registerCIPrTestCommand() *cli.Command {
	command := newCIPrTestCommand()
	CICommand.Commands = append(CICommand.Commands, command)
	return command
}

func newCIPrTestCommand() *cli.Command {
	return &cli.Command{
		Name:  "pr-test",
		Usage: "Run typed pull-request test workflow operations",
		Commands: []*cli.Command{
			newCIPrScopeCommand(),
			newCIPrDiscoverCommand(),
			newCIPrPlanIntegerCommand(),
			newCIPrPlanCopaCommand(),
			newCIPrTrivyCacheKeyCommand(),
			newCIPrIntegerBatchCommand(),
			newCIPrCopaMetadataCommand(),
			newCIPrCopaPinCommand(),
			newCIPrAggregateCommand(),
		},
	}
}
