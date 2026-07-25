package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/verity-org/verity/internal/ci"
)

func newCIIntegerImageCommand() *cli.Command {
	return &cli.Command{
		Name:  "integer-image",
		Usage: "Run Go-owned Integer image publication operations",
		Commands: []*cli.Command{
			{
				Name:  "test-packages",
				Usage: "Test every staged Integer package for one architecture",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "arch", Required: true},
					&cli.StringFlag{Name: "workspace", Value: "."},
					&cli.DurationFlag{Name: "timeout", Value: 30 * time.Minute},
				},
				Action: runCIIntegerImageTestPackages,
			},
			{
				Name:  "publish",
				Usage: "Stage, zero-CVE scan, and promote one exact multi-arch image",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "image", Required: true},
					&cli.StringFlag{Name: "version", Required: true},
					&cli.StringFlag{Name: "type", Required: true},
					&cli.StringFlag{Name: "registry", Required: true},
					&cli.StringFlag{Name: "tags", Required: true},
					&cli.StringFlag{Name: "title"},
					&cli.StringFlag{Name: "description"},
					&cli.StringFlag{Name: "config", Required: true},
					&cli.StringFlag{Name: "workspace", Value: "."},
					&cli.StringFlag{Name: "source-sha", Required: true},
					&cli.Uint64Flag{Name: "run-id", Required: true},
					&cli.Uint64Flag{Name: "run-attempt", Required: true},
					&cli.StringFlag{Name: "publication-id", Required: true},
					&cli.BoolFlag{Name: "melange"},
					&cli.StringFlag{Name: "github-output"},
				},
				Action: runCIIntegerImagePublish,
			},
		},
	}
}

func runCIIntegerImageTestPackages(ctx context.Context, command *cli.Command) error {
	return ci.TestIntegerPackages(ctx, &ci.IntegerPackageTestOptions{
		Architecture: ci.IntegerArchitecture(command.String("arch")),
		Workspace:    command.String("workspace"),
		Timeout:      command.Duration("timeout"),
	})
}

func runCIIntegerImagePublish(ctx context.Context, command *cli.Command) error {
	digest, err := ci.PublishIntegerImage(ctx, &ci.IntegerImagePublishOptions{
		Image:         command.String("image"),
		Version:       command.String("version"),
		Type:          command.String("type"),
		Registry:      command.String("registry"),
		Tags:          command.String("tags"),
		Title:         command.String("title"),
		Description:   command.String("description"),
		ConfigPath:    command.String("config"),
		Workspace:     command.String("workspace"),
		SourceSHA:     command.String("source-sha"),
		RunID:         command.Uint64("run-id"),
		RunAttempt:    command.Uint64("run-attempt"),
		PublicationID: command.String("publication-id"),
		Melange:       command.Bool("melange"),
		CreatedAt:     time.Now().UTC(),
	})
	if err != nil {
		return err
	}
	if outputPath := command.String("github-output"); outputPath != "" {
		return appendIntegerImageDigest(outputPath, command.String("publication-id"), digest)
	}
	return nil
}

func appendIntegerImageDigest(path, publicationID, digest string) (err error) {
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open Integer image output %q: %w", path, err)
	}
	defer func() {
		err = errors.Join(err, file.Close())
	}()
	if _, err := fmt.Fprintf(file, "publication_id=%s\ndigest=%s\n", publicationID, digest); err != nil {
		return fmt.Errorf("write Integer image output %q: %w", path, err)
	}
	return nil
}
