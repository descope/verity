package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/urfave/cli/v3"

	"github.com/verity-org/verity/internal/ci"
)

func newCIIntegerComponentCommand() *cli.Command {
	return &cli.Command{
		Name:  "component",
		Usage: "Validate and stage one exact Integer package component",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "plan", Required: true},
			&cli.StringFlag{Name: "target", Required: true},
			&cli.StringFlag{Name: "packages-dir", Required: true},
			&cli.StringFlag{Name: "output-dir", Required: true},
		},
		Action: runCIIntegerComponent,
	}
}

func runCIIntegerComponent(ctx context.Context, command *cli.Command) error {
	plan, err := readIntegerBatchPlan(command.String("plan"))
	if err != nil {
		return err
	}
	_, err = ci.StageIntegerComponent(ctx, &ci.IntegerComponentOptions{
		Plan:        &plan,
		TargetID:    command.String("target"),
		PackagesDir: command.String("packages-dir"),
		OutputDir:   command.String("output-dir"),
	})
	return err
}

func newCIIntegerShardCommand() *cli.Command {
	return &cli.Command{
		Name:  "shard",
		Usage: "Aggregate exact Integer components for one shard",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "plan", Required: true},
			&cli.StringFlag{Name: "shard", Required: true},
			&cli.StringFlag{Name: "components-dir", Required: true},
			&cli.StringFlag{Name: "output-dir", Required: true},
			&cli.StringFlag{Name: "inventory-output", Required: true},
		},
		Action: runCIIntegerShard,
	}
}

func runCIIntegerShard(ctx context.Context, command *cli.Command) error {
	plan, err := readIntegerBatchPlan(command.String("plan"))
	if err != nil {
		return err
	}
	componentDirs, err := integerManifestDirectories(command.String("components-dir"), ci.IntegerComponentManifestName)
	if err != nil {
		return err
	}
	inventory, err := ci.AggregateIntegerShard(ctx, &ci.IntegerShardOptions{
		Plan:          &plan,
		Shard:         command.String("shard"),
		ComponentDirs: componentDirs,
		OutputDir:     command.String("output-dir"),
	})
	if err != nil {
		return err
	}
	data, err := ci.MarshalIntegerShardInventory(&inventory)
	if err != nil {
		return err
	}
	return writeIntegerBatchFile(command.String("inventory-output"), data)
}

func integerManifestDirectories(root, manifestName string) ([]string, error) {
	var directories []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && entry.Name() == manifestName {
			directories = append(directories, filepath.Dir(path))
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("scan Integer manifests in %q: %w", root, err)
	}
	return directories, nil
}
