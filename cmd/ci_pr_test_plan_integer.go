package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/verity-org/verity/internal/ci"
	"github.com/verity-org/verity/internal/integer/apkindex"
)

func newCIPrPlanIntegerCommand() *cli.Command {
	return &cli.Command{
		Name:  "plan-integer",
		Usage: "Discover changed Integer variants and emit native architecture matrices",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "base-sha", Required: true},
			&cli.StringFlag{Name: "head-sha", Required: true},
			&cli.StringFlag{Name: "repo-root", Value: "."},
			&cli.StringFlag{Name: "temp-dir", Value: os.TempDir()},
			&cli.StringFlag{Name: "integer-config", Value: "integer.yaml"},
			&cli.StringFlag{Name: "images-dir", Value: "images"},
			&cli.StringFlag{Name: "apkindex-url", Value: apkindex.DefaultAPKINDEXURL},
			&cli.StringFlag{Name: "cache-dir"},
			&cli.StringFlag{Name: "gen-dir"},
			&cli.StringFlag{Name: "github-output", Required: true},
		},
		Action: runCIPrPlanInteger,
	}
}

func runCIPrPlanInteger(ctx context.Context, command *cli.Command) error {
	paths, err := loadPRChangedPaths(ctx, prChangedPathRequest{
		BaseSHA: command.String("base-sha"), HeadSHA: command.String("head-sha"), RepoRoot: command.String("repo-root"),
	})
	if err != nil {
		return err
	}
	root, err := os.MkdirTemp(command.String("temp-dir"), "verity-pr-integer-")
	if err != nil {
		return fmt.Errorf("create Integer plan directory: %w", err)
	}
	defer os.RemoveAll(root)

	base, err := stagePRIntegerBaseInputs(ctx, prIntegerBaseRequest{
		RepoRoot: command.String("repo-root"), BaseSHA: command.String("base-sha"), TempRoot: root, ChangedPaths: paths,
	})
	if err != nil {
		return err
	}

	genDir := command.String("gen-dir")
	if genDir == "" {
		genDir = filepath.Join(root, "integer-gen")
	}
	plan, err := ci.PlanIntegerPR(&ci.IntegerPROptions{
		ChangedFiles: paths, RepoRoot: command.String("repo-root"), BaseLockPath: base.LockPath, BaseImagesDir: base.ImagesDir,
		ConfigPath: command.String("integer-config"), ImagesDir: command.String("images-dir"), APKIndexURL: command.String("apkindex-url"),
		CacheDir: command.String("cache-dir"), GenDir: genDir,
	})
	if err != nil {
		return fmt.Errorf("plan Integer PR tests: %w", err)
	}
	return emitPRIntegerPlan(command, plan)
}

type prIntegerBaseRequest struct {
	RepoRoot     string
	BaseSHA      string
	TempRoot     string
	ChangedPaths []string
}

type prIntegerBaseInputs struct {
	LockPath  string
	ImagesDir string
}

func stagePRIntegerBaseInputs(ctx context.Context, request prIntegerBaseRequest) (prIntegerBaseInputs, error) {
	inputs := prIntegerBaseInputs{
		LockPath:  filepath.Join(request.TempRoot, "base-upstream.lock.json"),
		ImagesDir: filepath.Join(request.TempRoot, "base-images"),
	}
	data, ok, err := gitShowPRFile(ctx, prGitShowRequest{
		RepoRoot: request.RepoRoot, Revision: request.BaseSHA, Path: "packages/upstream.lock.json",
	})
	if err != nil {
		return prIntegerBaseInputs{}, err
	}
	if !ok {
		data = []byte("{\"packages\":{},\"pipeline_files\":{}}\n")
	}
	if err := writePRFile(inputs.LockPath, data); err != nil {
		return prIntegerBaseInputs{}, err
	}
	if err := os.MkdirAll(inputs.ImagesDir, 0o755); err != nil {
		return prIntegerBaseInputs{}, fmt.Errorf("create base images directory: %w", err)
	}
	for _, path := range request.ChangedPaths {
		if err := stagePRIntegerBaseImage(ctx, prIntegerBaseImageRequest{
			RepoRoot: request.RepoRoot, BaseSHA: request.BaseSHA, ImagesDir: inputs.ImagesDir, Path: path,
		}); err != nil {
			return prIntegerBaseInputs{}, err
		}
	}
	return inputs, nil
}

type prIntegerBaseImageRequest struct {
	RepoRoot  string
	BaseSHA   string
	ImagesDir string
	Path      string
}

func stagePRIntegerBaseImage(ctx context.Context, request prIntegerBaseImageRequest) error {
	if !strings.HasPrefix(request.Path, "images/") || !strings.HasSuffix(request.Path, ".yaml") {
		return nil
	}
	relative := filepath.Clean(filepath.FromSlash(strings.TrimPrefix(request.Path, "images/")))
	if relative == "." || filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%w: unsafe changed image path %q", errPRCommandFailed, request.Path)
	}
	data, ok, err := gitShowPRFile(ctx, prGitShowRequest{
		RepoRoot: request.RepoRoot, Revision: request.BaseSHA, Path: request.Path,
	})
	if err != nil || !ok {
		return err
	}
	return writePRFile(filepath.Join(request.ImagesDir, relative), data)
}

func emitPRIntegerPlan(command *cli.Command, plan ci.Plan) error {
	smoke := ci.Matrix{}
	if plan.SmokeMatrix != nil {
		smoke = *plan.SmokeMatrix
	}
	strictBatches, err := newPRIntegerBatchMatrix(plan.Matrix)
	if err != nil {
		return err
	}
	smokeBatches, err := newPRIntegerBatchMatrix(smoke)
	if err != nil {
		return err
	}
	expected, err := prExpectedIntegerMatrix(plan.Matrix)
	if err != nil {
		return err
	}
	expectedSmoke, err := prExpectedIntegerMatrix(smoke)
	if err != nil {
		return err
	}
	values := make([][2]string, 0, 6)
	for _, value := range []struct {
		name string
		data any
	}{
		{name: "matrix", data: strictBatches},
		{name: "smoke-matrix", data: smokeBatches},
		{name: "expected-matrix", data: expected},
		{name: "expected-smoke-matrix", data: expectedSmoke},
	} {
		encoded, err := marshalPRJSON(value.data)
		if err != nil {
			return err
		}
		values = append(values, [2]string{value.name, encoded})
	}
	values = append(
		values,
		[2]string{"has-changes", strconv.FormatBool(plan.HasChanges)},
		[2]string{"smoke-has-changes", strconv.FormatBool(len(smoke.Include) > 0)},
	)
	if err := appendPRGitHubValues(command.String("github-output"), values); err != nil {
		return err
	}
	_, err = fmt.Fprintf(command.Writer, "Integer matrix: %d strict builds and %d smoke-only variants in %d native legs\n", len(plan.Matrix.Include), len(smoke.Include), len(strictBatches.Include))
	if err != nil {
		return fmt.Errorf("write Integer plan summary: %w", err)
	}
	return nil
}
