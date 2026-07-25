package cmd

import (
	"github.com/urfave/cli/v3"

	workflowopscommand "github.com/verity-org/verity/internal/ci/workflowops/command"
)

var ciWorkflowOpsCommand = registerCIWorkflowOpsCommand()

func registerCIWorkflowOpsCommand() *cli.Command {
	command := workflowopscommand.New()
	CICommand.Commands = append(CICommand.Commands, command)
	return command
}
