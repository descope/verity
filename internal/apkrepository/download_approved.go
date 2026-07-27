package apkrepository

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var batchIDPattern = regexp.MustCompile(`^(\d+)-([1-9]\d*)$`)

type DownloadApprovedOptions struct {
	BatchID    string
	OutputDir  string
	Repository string
	Stdout     io.Writer
	runner     commandRunner
}

func DownloadApproved(ctx context.Context, options *DownloadApprovedOptions) error {
	if options == nil {
		return errOptionsRequired
	}
	matches := batchIDPattern.FindStringSubmatch(options.BatchID)
	if matches == nil {
		return fmt.Errorf("%w: %s", errInvalidBatchID, options.BatchID)
	}
	if err := validateOutputDirectory(options.OutputDir); err != nil {
		return err
	}
	if strings.TrimSpace(options.Repository) == "" {
		return errRepositoryEnvironmentRequired
	}
	runner := options.runner
	if runner == nil {
		runner = execCommandRunner{}
	}
	if err := os.RemoveAll(options.OutputDir); err != nil {
		return fmt.Errorf("clear artifact output: %w", err)
	}
	if err := os.MkdirAll(options.OutputDir, 0o755); err != nil {
		return fmt.Errorf("create artifact output: %w", err)
	}
	_, err := runRequired(ctx, runner, &command{
		name: "gh",
		args: []string{
			"run", "download", matches[1],
			"--repo", options.Repository,
			"--pattern", "apk-repository-" + options.BatchID + "-*",
			"--dir", options.OutputDir,
		},
	})
	if err != nil {
		return fmt.Errorf("download Integer artifacts: %w", err)
	}
	packages, err := findDownloadedPackages(options.OutputDir)
	if err != nil {
		return err
	}
	if len(packages) == 0 {
		return fmt.Errorf("%w: %s", errNoApprovedArtifacts, options.BatchID)
	}
	signerWorkflow := "github.com/" + options.Repository + "/.github/workflows/integer-build-image-reusable.yaml"
	for _, packagePath := range packages {
		_, err := runRequired(ctx, runner, &command{
			name: "gh",
			args: []string{
				"attestation", "verify", packagePath,
				"--repo", options.Repository,
				"--signer-workflow", signerWorkflow,
				"--source-ref", "refs/heads/main",
				"--deny-self-hosted-runners",
			},
		})
		if err != nil {
			return fmt.Errorf("verify attestation for %s: %w", packagePath, err)
		}
	}
	_, err = fmt.Fprintf(writerOrDiscard(options.Stdout), "Downloaded and verified %d approved APK packages from Integer batch %s\n", len(packages), options.BatchID)
	return err
}

func findDownloadedPackages(root string) ([]string, error) {
	packages := make([]string, 0)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type().IsRegular() && strings.HasSuffix(entry.Name(), ".apk") {
			packages = append(packages, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan downloaded APKs: %w", err)
	}
	sort.Strings(packages)
	return packages, nil
}
