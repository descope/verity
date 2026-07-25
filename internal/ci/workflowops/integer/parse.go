package integer

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"

	"github.com/verity-org/verity/internal/ci/workflowops/strictjson"
)

var (
	errEmptyIdentity = errors.New("required identity field is empty")
	errInvalidBatch  = errors.New("invalid batch ID")
	errInvalidRunID  = errors.New("invalid run ID")
	errInvalidStatus = errors.New("unsupported status")
	errNegativeShard = errors.New("negative shard")
)

func parseInputs(ctx context.Context, options *Options) (parsedInput, error) {
	planContent, err := os.ReadFile(options.ExpectedPath)
	if err != nil {
		return parsedInput{}, fmt.Errorf("read Integer plan %q: %w", options.ExpectedPath, err)
	}
	var plan []planEntry
	if err := strictjson.Decode(planContent, &plan); err != nil {
		return parsedInput{}, fmt.Errorf("%w: %w", ErrInvalidPlan, err)
	}
	seen := make(map[string]struct{}, len(plan))
	for _, entry := range plan {
		if entry.Name == "" || entry.Version == "" || entry.Type == "" {
			return parsedInput{}, fmt.Errorf("%w: empty plan identity", ErrInvalidPlan)
		}
		key := identity(entry.Name, entry.Version, entry.Type)
		if _, exists := seen[key]; exists {
			return parsedInput{}, fmt.Errorf("%w: duplicate %s", ErrInvalidPlan, key)
		}
		seen[key] = struct{}{}
	}

	paths, err := reportPaths(options.ResultsDir)
	if err != nil {
		return parsedInput{}, err
	}
	reports := make([]childReport, 0, len(paths))
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return parsedInput{}, err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return parsedInput{}, fmt.Errorf("read child report %q: %w", path, err)
		}
		var report childReport
		if err := strictjson.Decode(content, &report); err != nil {
			return parsedInput{}, fmt.Errorf("%w: %s: %w", ErrInvalidChildReport, path, err)
		}
		if err := validateReport(&report); err != nil {
			return parsedInput{}, fmt.Errorf("%w: %s: %w", ErrInvalidChildReport, path, err)
		}
		reports = append(reports, report)
	}
	return parsedInput{plan: plan, reports: reports}, nil
}

func reportPaths(root string) ([]string, error) {
	if _, err := os.Stat(root); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("stat Integer results %q: %w", root, err)
	}
	var paths []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && entry.Name() == "report.json" {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk Integer results %q: %w", root, err)
	}
	sort.Strings(paths)
	return paths, nil
}

func validateReport(report *childReport) error {
	if report.Image == "" || report.Version == "" || report.Type == "" || report.RunID == "" || report.BatchID == "" || report.SourceSHA == "" || report.Repository == "" {
		return errEmptyIdentity
	}
	if err := validateReportIdentity(report); err != nil {
		return err
	}
	if report.Status != "success" && report.Status != "failure" {
		return fmt.Errorf("%w %q", errInvalidStatus, report.Status)
	}
	if report.Shard < 0 {
		return errNegativeShard
	}
	return nil
}

func validateReportIdentity(report *childReport) error {
	if !batchPattern.MatchString(report.BatchID) {
		return errInvalidBatch
	}
	runID, err := strconv.ParseInt(report.RunID, 10, 64)
	if err != nil || runID < 1 {
		return errInvalidRunID
	}
	if report.RunAttempt < 1 || !sourcePattern.MatchString(report.SourceSHA) {
		return errEmptyIdentity
	}
	return nil
}

func identity(image, version, variant string) string {
	return image + "\x00" + version + "\x00" + variant
}
