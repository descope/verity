package cmd

import (
	"github.com/urfave/cli/v3"

	repositoryops "github.com/verity-org/verity/internal/ci/repositoryops"
)

var ciRepositoryOpsCommand = func() *cli.Command {
	command := repositoryops.NewCLICommand()
	CICommand.Commands = append(CICommand.Commands, command)
	return command
}()
