package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/verity-org/verity/internal/discovery"
)

func newCIPrDiscoverCommand() *cli.Command {
	return &cli.Command{
		Name:  "discover",
		Usage: "Run Copa discovery and emit a typed JSON artifact summary",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "config", Value: "copa-config.yaml"},
			&cli.StringFlag{Name: "target-registry"},
			&cli.StringFlag{Name: "output", Value: "images.json"},
			&cli.StringFlag{Name: prVerityFlagName, Value: "./verity"},
			&cli.StringFlag{Name: "repo-root", Value: "."},
		},
		Action: runCIPrDiscover,
	}
}

func runCIPrDiscover(ctx context.Context, command *cli.Command) error {
	args := []string{"discover", "--config", command.String("config")}
	if registry := strings.TrimSpace(command.String("target-registry")); registry != "" {
		args = append(args, "--target-registry", registry)
	}
	result, err := requirePRCommand(ctx, &prCommandRequest{
		Name:   command.String(prVerityFlagName),
		Args:   args,
		Dir:    command.String("repo-root"),
		Stderr: command.ErrWriter,
	})
	if err != nil {
		return fmt.Errorf("discover Copa images: %w", err)
	}
	var images []discovery.DiscoveredImage
	if err := json.Unmarshal(result.Stdout, &images); err != nil {
		return fmt.Errorf("parse discovered images: %w", err)
	}
	if err := os.WriteFile(command.String("output"), result.Stdout, 0o600); err != nil {
		return fmt.Errorf("write discovery output: %w", err)
	}
	if _, err := fmt.Fprintf(command.Writer, "Discovered %d image+tag combos\n", len(images)); err != nil {
		return fmt.Errorf("write discovery summary: %w", err)
	}
	for _, image := range images {
		if _, err := fmt.Fprintf(command.Writer, "  - %s: %s\n", image.Name, image.Source); err != nil {
			return fmt.Errorf("write discovery image: %w", err)
		}
	}
	return nil
}
