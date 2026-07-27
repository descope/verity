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

const integerShardManifestName = "shard-manifest.json"

func newCIIntegerFinalizeShardCommand() *cli.Command {
	return &cli.Command{
		Name:  "finalize-shard",
		Usage: "Bind an approved package artifact name and digest to a shard manifest",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "inventory", Required: true},
			&cli.StringFlag{Name: "publication-id", Required: true},
			&cli.StringFlag{Name: "artifact-name", Required: true},
			&cli.StringFlag{Name: "artifact-digest", Required: true},
			&cli.StringFlag{Name: "output", Required: true},
		},
		Action: runCIIntegerFinalizeShard,
	}
}

func runCIIntegerFinalizeShard(_ context.Context, command *cli.Command) error {
	data, err := os.ReadFile(command.String("inventory"))
	if err != nil {
		return fmt.Errorf("read Integer shard inventory: %w", err)
	}
	inventory, err := ci.ParseIntegerShardInventory(data)
	if err != nil {
		return err
	}
	manifest, err := ci.FinalizeIntegerShard(&inventory, ci.IntegerArtifactRef{
		PublicationID: command.String("publication-id"), Name: command.String("artifact-name"), Digest: command.String("artifact-digest"),
	})
	if err != nil {
		return err
	}
	data, err = ci.MarshalIntegerShardManifest(&manifest)
	if err != nil {
		return err
	}
	return writeIntegerBatchFile(command.String("output"), data)
}

func newCIIntegerAggregateCommand() *cli.Command {
	return &cli.Command{
		Name:  "aggregate",
		Usage: "Aggregate complete exact Integer shard manifests",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "plan", Required: true},
			&cli.StringFlag{Name: "shards-dir", Required: true},
			&cli.StringFlag{Name: "output", Required: true},
		},
		Action: runCIIntegerAggregate,
	}
}

func runCIIntegerAggregate(_ context.Context, command *cli.Command) error {
	plan, err := readIntegerBatchPlan(command.String("plan"))
	if err != nil {
		return err
	}
	shards, err := readIntegerShardManifests(command.String("shards-dir"))
	if err != nil {
		return err
	}
	manifest, err := ci.AggregateIntegerBatch(&plan, shards)
	if err != nil {
		return err
	}
	data, err := ci.MarshalIntegerBatchManifest(&manifest)
	if err != nil {
		return err
	}
	return writeIntegerBatchFile(command.String("output"), data)
}

func readIntegerShardManifests(root string) (manifests []ci.IntegerShardManifest, err error) {
	rootDirectory, err := os.OpenRoot(root)
	if errors.Is(err, os.ErrNotExist) {
		return []ci.IntegerShardManifest{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open Integer shard directory: %w", err)
	}
	defer func() {
		err = errors.Join(err, rootDirectory.Close())
	}()
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || entry.Name() != integerShardManifestName {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return fmt.Errorf("make shard manifest path relative: %w", err)
		}
		data, err := rootDirectory.ReadFile(relative)
		if err != nil {
			return err
		}
		manifest, err := ci.ParseIntegerShardManifest(data)
		if err != nil {
			return err
		}
		manifests = append(manifests, manifest)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read Integer shard manifests: %w", err)
	}
	return manifests, nil
}
