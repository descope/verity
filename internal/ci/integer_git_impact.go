package ci

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type IntegerGitImpact struct {
	ChangedFiles  []string
	BaseLockPath  string
	BaseImagesDir string
}

type IntegerGitImpactOptions struct {
	Repository string
	BaseSHA    string
	HeadSHA    string
	OutputDir  string
}

func LoadIntegerGitImpact(ctx context.Context, options *IntegerGitImpactOptions) (IntegerGitImpact, error) {
	if err := ctx.Err(); err != nil {
		return IntegerGitImpact{}, err
	}
	if options == nil || options.Repository == "" || options.OutputDir == "" ||
		!integerSourceSHAPattern.MatchString(options.BaseSHA) || !integerSourceSHAPattern.MatchString(options.HeadSHA) {
		return IntegerGitImpact{}, fmt.Errorf("%w: Git impact identity", ErrIntegerBatchPlan)
	}
	for _, revision := range []string{options.BaseSHA, options.HeadSHA} {
		if _, err := runIntegerGitCommand(ctx, options.Repository, "cat-file", "-e", revision+"^{commit}"); err != nil {
			return IntegerGitImpact{}, fmt.Errorf("%w: revision %s: %w", ErrIntegerBatchPlan, revision, err)
		}
	}
	changedFiles, err := loadIntegerGitChangedFiles(ctx, options)
	if err != nil {
		return IntegerGitImpact{}, err
	}
	impact := IntegerGitImpact{
		ChangedFiles:  changedFiles,
		BaseImagesDir: filepath.Join(options.OutputDir, "base-images"),
	}
	if err := materializeIntegerGitBase(ctx, options, &impact); err != nil {
		return IntegerGitImpact{}, err
	}
	return impact, nil
}

func loadIntegerGitChangedFiles(ctx context.Context, options *IntegerGitImpactOptions) ([]string, error) {
	output, err := runIntegerGitCommand(ctx, options.Repository, "diff", "--name-only", "--no-renames", options.BaseSHA, options.HeadSHA)
	if err != nil {
		return nil, fmt.Errorf("load committed Integer changes: %w", err)
	}
	var changedFiles []string
	for line := range strings.SplitSeq(string(output), "\n") {
		line = filepath.ToSlash(strings.TrimSpace(line))
		if line == "" {
			continue
		}
		if filepath.IsAbs(line) || strings.HasPrefix(line, "../") || strings.Contains(line, "/../") {
			return nil, fmt.Errorf("%w: unsafe changed path %q", ErrIntegerBatchPlan, line)
		}
		changedFiles = append(changedFiles, line)
	}
	return changedFiles, nil
}

func materializeIntegerGitBase(ctx context.Context, options *IntegerGitImpactOptions, impact *IntegerGitImpact) error {
	if containsNormalizedPath(impact.ChangedFiles, "packages/upstream.lock.json") {
		impact.BaseLockPath = filepath.Join(options.OutputDir, "base-upstream.lock.json")
		if err := materializeIntegerGitFile(ctx, integerGitFileRequest{
			repository: options.Repository, revision: options.BaseSHA, path: "packages/upstream.lock.json",
			destination: impact.BaseLockPath, required: true,
		}); err != nil {
			return err
		}
	}
	for _, changed := range impact.ChangedFiles {
		if !strings.HasPrefix(changed, "images/") || strings.HasPrefix(changed, "images/_base/") {
			continue
		}
		relative := strings.TrimPrefix(changed, "images/")
		destination := filepath.Join(impact.BaseImagesDir, filepath.FromSlash(relative))
		if err := materializeIntegerGitFile(ctx, integerGitFileRequest{
			repository: options.Repository, revision: options.BaseSHA, path: changed,
			destination: destination,
		}); err != nil {
			return err
		}
	}
	return nil
}

type integerGitFileRequest struct {
	repository  string
	revision    string
	path        string
	destination string
	required    bool
}

func materializeIntegerGitFile(ctx context.Context, request integerGitFileRequest) error {
	object := request.revision + ":" + request.path
	if _, err := runIntegerGitCommand(ctx, request.repository, "cat-file", "-e", object); err != nil {
		if !request.required && errors.Is(err, errIntegerGitCommand) {
			return nil
		}
		return fmt.Errorf("%w: base file %s: %w", ErrIntegerBatchPlan, request.path, err)
	}
	data, err := runIntegerGitCommand(ctx, request.repository, "show", object)
	if err != nil {
		return fmt.Errorf("read base file %s: %w", request.path, err)
	}
	if err := os.MkdirAll(filepath.Dir(request.destination), 0o755); err != nil {
		return fmt.Errorf("create base file directory: %w", err)
	}
	if err := os.WriteFile(request.destination, data, 0o600); err != nil {
		return fmt.Errorf("write base file %s: %w", request.path, err)
	}
	return nil
}

var errIntegerGitCommand = errors.New("integer Git command failed")

func runIntegerGitCommand(ctx context.Context, repository string, arguments ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, "git", arguments...)
	command.Dir = repository
	output, err := command.Output()
	if err == nil {
		return output, nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, ctxErr
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		return nil, fmt.Errorf("%w: git %s exited %d: %s", errIntegerGitCommand, strings.Join(arguments, " "), exitError.ExitCode(), strings.TrimSpace(string(exitError.Stderr)))
	}
	return nil, fmt.Errorf("run git %s: %w", strings.Join(arguments, " "), err)
}
