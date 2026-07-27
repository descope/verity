package chartops

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/verity-org/verity/internal/ci"
)

var ErrInvalidMatrixInput = errors.New("invalid chart matrix input")

type MatrixInput struct {
	RepoRoot     string
	EventName    string
	BaseSHA      string
	HeadSHA      string
	InputChart   string
	ChartsFile   string
	VerityConfig string
	ValuesDir    string
}

type MatrixResult struct {
	Charts []string
	Strict bool
}

func BuildMatrix(ctx context.Context, input *MatrixInput) (result MatrixResult, retErr error) {
	if input == nil {
		return MatrixResult{}, fmt.Errorf("%w: input is required", ErrInvalidMatrixInput)
	}
	repoRoot := defaultValue(input.RepoRoot, ".")
	changedFiles, baseChartsFile, cleanup, err := chartInputs(ctx, repoRoot, input)
	if err != nil {
		return MatrixResult{}, err
	}
	defer func() { retErr = errors.Join(retErr, cleanup()) }()

	plan, err := ci.PlanCharts(&ci.ChartOptions{
		EventName: input.EventName, InputChart: input.InputChart, ChangedFiles: changedFiles,
		ChartsFile: repoPath(repoRoot, input.ChartsFile, "Chart.yaml"), BaseChartsFile: baseChartsFile,
		VerityConfig: repoPath(repoRoot, input.VerityConfig, "verity.yaml"),
		ValuesDir:    repoPath(repoRoot, input.ValuesDir, filepath.Join("test", "chart-integration", "values")),
	})
	if err != nil {
		return MatrixResult{}, fmt.Errorf("plan chart matrix: %w", err)
	}

	charts := make([]string, 0, len(plan.Matrix.Include))
	for _, entry := range plan.Matrix.Include {
		chart := strings.TrimSpace(entry["chart"])
		if chart == "" {
			return MatrixResult{}, fmt.Errorf("%w: matrix entry is missing chart", ErrInvalidMatrixInput)
		}
		charts = append(charts, chart)
	}
	return MatrixResult{Charts: charts, Strict: plan.Strict}, nil
}

func chartInputs(ctx context.Context, repoRoot string, input *MatrixInput) (
	changedFiles []string,
	baseChartsFile string,
	cleanup func() error,
	retErr error,
) {
	if input.EventName != "pull_request" {
		return nil, "", func() error { return nil }, nil
	}
	if strings.TrimSpace(input.BaseSHA) == "" || strings.TrimSpace(input.HeadSHA) == "" {
		return nil, "", func() error { return nil }, fmt.Errorf("%w: pull requests require base and head SHAs", ErrInvalidMatrixInput)
	}

	diff, err := gitOutput(ctx, repoRoot, "diff", "--name-only", input.BaseSHA+"..."+input.HeadSHA, "--")
	if err != nil {
		return nil, "", func() error { return nil }, err
	}
	changedFiles = nonemptyLines(string(diff))
	baseChart, showErr := gitOutput(ctx, repoRoot, "show", input.BaseSHA+":Chart.yaml")
	if showErr != nil {
		if _, err := gitOutput(ctx, repoRoot, "cat-file", "-e", input.BaseSHA+"^{commit}"); err != nil {
			return nil, "", func() error { return nil }, showErr
		}
		baseChart = []byte("dependencies: []\n")
	}

	file, err := os.CreateTemp("", "verity-base-chart-*.yaml")
	if err != nil {
		return nil, "", func() error { return nil }, fmt.Errorf("create base Chart.yaml: %w", err)
	}
	path := file.Name()
	cleanup = func() error {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove base Chart.yaml %q: %w", path, err)
		}
		return nil
	}
	if _, err := file.Write(baseChart); err != nil {
		writeErr := fmt.Errorf("write base Chart.yaml: %w", err)
		if closeErr := file.Close(); closeErr != nil {
			writeErr = errors.Join(writeErr, fmt.Errorf("close base Chart.yaml: %w", closeErr))
		}
		return nil, "", func() error { return nil }, errors.Join(writeErr, cleanup())
	}
	if err := file.Close(); err != nil {
		return nil, "", func() error { return nil }, errors.Join(fmt.Errorf("close base Chart.yaml: %w", err), cleanup())
	}
	return changedFiles, path, cleanup, nil
}

func gitOutput(ctx context.Context, repoRoot string, args ...string) ([]byte, error) {
	commandArgs := append([]string{"-C", repoRoot}, args...)
	output, err := exec.CommandContext(ctx, "git", commandArgs...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return output, nil
}

func nonemptyLines(value string) []string {
	var lines []string
	for line := range strings.SplitSeq(value, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func repoPath(root, value, fallback string) string {
	path := defaultValue(value, fallback)
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(root, path)
}

func defaultValue(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
