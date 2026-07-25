package cmd

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/urfave/cli/v3"

	"github.com/verity-org/verity/internal/buildmetadata"
)

// VersionCommand prints concise human or machine-readable build metadata.
var VersionCommand = &cli.Command{
	Name:  "version",
	Usage: "Print Verity build metadata",
	Flags: []cli.Flag{
		&cli.BoolFlag{Name: "json", Usage: "Emit machine-readable metadata"},
	},
	Action: runVersion,
}

func runVersion(_ context.Context, command *cli.Command) error {
	info, err := buildmetadata.CurrentValidated()
	if err != nil {
		return fmt.Errorf("verity version: %w", err)
	}
	if command.Bool("json") {
		return writeVersionJSON(os.Stdout, &info)
	}
	return writeHumanVersion(os.Stdout, &info)
}

func writeVersionJSON(writer io.Writer, info *buildmetadata.Info) error {
	data, err := buildmetadata.MarshalInfo(*info)
	if err != nil {
		return fmt.Errorf("write version JSON: %w", err)
	}
	if _, err := writer.Write(data); err != nil {
		return fmt.Errorf("write version JSON: %w", err)
	}
	return nil
}

func writeHumanVersion(writer io.Writer, info *buildmetadata.Info) error {
	if _, err := fmt.Fprintf(writer, "verity %s\n", info.Version); err != nil {
		return fmt.Errorf("write version: %w", err)
	}
	return nil
}
