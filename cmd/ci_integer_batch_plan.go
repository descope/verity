package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/verity-org/verity/internal/ci"
	"github.com/verity-org/verity/internal/integer/apkindex"
)

type integerShardOutput struct {
	Shard          string `json:"shard"`
	Count          int    `json:"count"`
	ComponentCount int    `json:"component_count"`
	Entries        string `json:"entries"`
}

type integerExpectedTarget struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Type    string `json:"type"`
}

type integerPlanImpact struct {
	changedFiles  []string
	baseLockPath  string
	baseImagesDir string
	temporaryDir  string
}

func newCIIntegerBatchPlanCommand() *cli.Command {
	return &cli.Command{
		Name:  "plan",
		Usage: "Plan an exact Integer production snapshot or recipe delta",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "event", Required: true},
			&cli.StringFlag{Name: "source-sha", Required: true},
			&cli.StringFlag{Name: "base-sha"},
			&cli.StringFlag{Name: "head-sha"},
			&cli.Uint64Flag{Name: "run-id", Required: true},
			&cli.Uint64Flag{Name: "run-attempt", Required: true},
			&cli.StringFlag{Name: "publication-id", Required: true},
			&cli.StringFlag{Name: "batch-id", Required: true},
			&cli.StringFlag{Name: "changed-files"},
			&cli.StringFlag{Name: "only", Usage: "Comma-separated image names"},
			&cli.BoolFlag{Name: "package-targets-only"},
			&cli.StringFlag{Name: "repo-root", Value: "."},
			&cli.StringFlag{Name: "base-upstream-lock"},
			&cli.StringFlag{Name: "base-images-dir"},
			&cli.StringFlag{Name: "integer-config", Value: "integer.yaml"},
			&cli.StringFlag{Name: "images-dir", Value: "images"},
			&cli.StringFlag{Name: "apkindex-url", Value: apkindex.DefaultAPKINDEXURL},
			&cli.StringFlag{Name: "cache-dir"},
			&cli.StringFlag{Name: "gen-dir"},
			&cli.StringFlag{Name: "plan-output", Required: true},
			&cli.StringFlag{Name: "expected-output", Required: true},
			&cli.StringFlag{Name: "github-output"},
		},
		Action: runCIIntegerBatchPlan,
	}
}

func runCIIntegerBatchPlan(ctx context.Context, command *cli.Command) (err error) {
	impact, err := resolveIntegerPlanImpact(ctx, command)
	if err != nil {
		return err
	}
	if impact.temporaryDir != "" {
		defer func() {
			err = errors.Join(err, os.RemoveAll(impact.temporaryDir))
		}()
	}
	plan, err := ci.PlanIntegerProduction(&ci.IntegerProductionOptions{
		Event:              ci.IntegerBatchEvent(command.String("event")),
		SourceSHA:          command.String("source-sha"),
		RunID:              command.Uint64("run-id"),
		RunAttempt:         command.Uint64("run-attempt"),
		PublicationID:      command.String("publication-id"),
		BatchID:            command.String("batch-id"),
		ChangedFiles:       impact.changedFiles,
		Only:               splitIntegerNames(command.String("only")),
		PackageTargetsOnly: command.Bool("package-targets-only"),
		RepoRoot:           command.String("repo-root"),
		BaseLockPath:       impact.baseLockPath,
		BaseImagesDir:      impact.baseImagesDir,
		ConfigPath:         command.String("integer-config"),
		ImagesDir:          command.String("images-dir"),
		APKIndexURL:        command.String("apkindex-url"),
		CacheDir:           command.String("cache-dir"),
		GenDir:             command.String("gen-dir"),
	})
	if err != nil {
		return err
	}
	data, err := ci.MarshalIntegerBatchPlan(&plan)
	if err != nil {
		return err
	}
	if err := writeIntegerBatchFile(command.String("plan-output"), data); err != nil {
		return err
	}
	if err := writeIntegerExpectedTargets(command.String("expected-output"), plan.Targets); err != nil {
		return err
	}
	if path := command.String("github-output"); path != "" {
		return appendIntegerPlanOutputs(path, &plan)
	}
	return nil
}

func resolveIntegerPlanImpact(ctx context.Context, command *cli.Command) (integerPlanImpact, error) {
	changed, err := readChangedFiles(command.String("changed-files"))
	if err != nil {
		return integerPlanImpact{}, err
	}
	impact := integerPlanImpact{
		changedFiles: changed, baseLockPath: command.String("base-upstream-lock"),
		baseImagesDir: command.String("base-images-dir"),
	}
	baseSHA := command.String("base-sha")
	headSHA := command.String("head-sha")
	if (baseSHA == "") != (headSHA == "") {
		return integerPlanImpact{}, fmt.Errorf("%w: base-sha and head-sha must be provided together", ci.ErrIntegerBatchPlan)
	}
	if baseSHA == "" {
		return impact, nil
	}
	if len(impact.changedFiles) != 0 || impact.baseLockPath != "" || impact.baseImagesDir != "" {
		return integerPlanImpact{}, fmt.Errorf("%w: Git range conflicts with precomputed impact inputs", ci.ErrIntegerBatchPlan)
	}
	if headSHA != command.String("source-sha") {
		return integerPlanImpact{}, fmt.Errorf("%w: head-sha does not match source-sha", ci.ErrIntegerBatchPlan)
	}
	impact.temporaryDir, err = os.MkdirTemp("", "verity-integer-impact-")
	if err != nil {
		return integerPlanImpact{}, fmt.Errorf("create Integer impact directory: %w", err)
	}
	gitImpact, err := ci.LoadIntegerGitImpact(ctx, &ci.IntegerGitImpactOptions{
		Repository: command.String("repo-root"), BaseSHA: baseSHA, HeadSHA: headSHA, OutputDir: impact.temporaryDir,
	})
	if err != nil {
		return integerPlanImpact{}, errors.Join(err, os.RemoveAll(impact.temporaryDir))
	}
	impact.changedFiles = gitImpact.ChangedFiles
	impact.baseLockPath = gitImpact.BaseLockPath
	impact.baseImagesDir = gitImpact.BaseImagesDir
	return impact, nil
}

func writeIntegerExpectedTargets(path string, targets []ci.IntegerBatchTarget) error {
	expected := make([]integerExpectedTarget, 0, len(targets))
	for index := range targets {
		target := &targets[index]
		expected = append(expected, integerExpectedTarget{Name: target.Name, Version: target.Version, Type: target.Type})
	}
	data, err := json.Marshal(expected)
	if err != nil {
		return fmt.Errorf("marshal Integer expected targets: %w", err)
	}
	return writeIntegerBatchFile(path, data)
}

func appendIntegerPlanOutputs(path string, plan *ci.IntegerBatchPlan) (err error) {
	shards, err := integerPlanShards(plan.Targets)
	if err != nil {
		return err
	}
	data, err := json.Marshal(shards)
	if err != nil {
		return fmt.Errorf("marshal Integer shard outputs: %w", err)
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open GitHub output %q: %w", path, err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("close GitHub output %q: %w", path, closeErr)
		}
	}()
	_, err = fmt.Fprintf(
		file,
		"count=%d\nshard_count=%d\nmode=%s\nevent=%s\nsource_sha=%s\nrun_id=%d\nrun_attempt=%d\npublication_id=%s\nbatch_id=%s\nshards<<__VERITY_INTEGER_SHARDS__\n%s\n__VERITY_INTEGER_SHARDS__\n",
		len(plan.Targets), len(shards), plan.Mode, plan.Event, plan.SourceSHA, plan.RunID, plan.RunAttempt, plan.PublicationID, plan.BatchID, data,
	)
	if err != nil {
		return fmt.Errorf("write GitHub Integer outputs: %w", err)
	}
	return nil
}

func integerPlanShards(targets []ci.IntegerBatchTarget) ([]integerShardOutput, error) {
	grouped := map[string][]ci.IntegerBatchTarget{}
	for index := range targets {
		target := &targets[index]
		grouped[target.Shard] = append(grouped[target.Shard], *target)
	}
	ids := make([]string, 0, len(grouped))
	for shard := range grouped {
		ids = append(ids, shard)
	}
	sort.Strings(ids)
	shards := make([]integerShardOutput, 0, len(ids))
	for _, shard := range ids {
		entries, err := json.Marshal(grouped[shard])
		if err != nil {
			return nil, fmt.Errorf("marshal Integer shard %s: %w", shard, err)
		}
		componentCount := 0
		for index := range grouped[shard] {
			if len(grouped[shard][index].PublishPackages) > 0 {
				componentCount++
			}
		}
		shards = append(shards, integerShardOutput{
			Shard: shard, Count: len(grouped[shard]), ComponentCount: componentCount, Entries: string(entries),
		})
	}
	return shards, nil
}

func splitIntegerNames(value string) []string {
	var names []string
	for name := range strings.SplitSeq(value, ",") {
		if name = strings.TrimSpace(name); name != "" {
			names = append(names, name)
		}
	}
	return names
}
