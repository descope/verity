package cmd

import (
	"github.com/urfave/cli/v3"

	"github.com/verity-org/verity/internal/ci/patchimage"
)

var ciPatchImageCommand = registerCIPatchImageCommand()

func registerCIPatchImageCommand() *cli.Command {
	command := patchimage.NewCommand()
	CICommand.Commands = append(CICommand.Commands, command)
	return command
}
