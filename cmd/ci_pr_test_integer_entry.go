package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const prIntegerSeverities = "UNKNOWN,LOW,MEDIUM,HIGH,CRITICAL"

func executePRIntegerEntry(
	ctx context.Context,
	deps *prIntegerDependencies,
	request *prIntegerBatchRequest,
	entry prIntegerEntry,
) (err error) {
	paths, err := preparePRIntegerEntry(ctx, deps, request, entry)
	if err != nil {
		return err
	}
	tarPath := filepath.Join(request.RepoRoot, fmt.Sprintf("image-%s-%s-%s-%s.tar", paths.safeImage, entry.Version, entry.Type, request.Architecture))
	if err := runPRIntegerBuild(ctx, deps, request, entry, tarPath); err != nil {
		return err
	}
	defer func() {
		if removeErr := os.Remove(tarPath); removeErr != nil && !os.IsNotExist(removeErr) {
			err = errors.Join(err, fmt.Errorf("remove Integer image archive: %w", removeErr))
		}
	}()
	loaded, err := loadPRIntegerImage(ctx, deps.Commands, prIntegerLoadRequest{
		TarPath: tarPath, Architecture: request.Architecture,
	})
	if err != nil {
		return err
	}
	if err := runPRIntegerRuntimeChecks(ctx, deps, request, entry, loaded); err != nil {
		return err
	}
	reportPath := filepath.Join(paths.reportDir, request.Architecture+".json")
	total, err := runPRIntegerTrivy(ctx, deps, request.RepoRoot, tarPath, reportPath)
	if err != nil {
		return err
	}
	fmt.Fprintf(deps.Stdout, "%s:%s-%s %s: Total vulnerabilities: %d\n", entry.Image, entry.Version, entry.Type, request.Architecture, total)
	marker := filepath.Join(paths.securityRoot, fmt.Sprintf(
		"%s-%s-%s-%s-%s.passed",
		request.Kind,
		paths.safeImage,
		entry.Version,
		entry.Type,
		request.Architecture,
	))
	if err := os.WriteFile(marker, nil, 0o600); err != nil {
		return fmt.Errorf("write Integer security marker: %w", err)
	}
	return nil
}

type prIntegerEntryPaths struct {
	safeImage    string
	reportDir    string
	securityRoot string
}

func preparePRIntegerEntry(
	ctx context.Context,
	deps *prIntegerDependencies,
	request *prIntegerBatchRequest,
	entry prIntegerEntry,
) (prIntegerEntryPaths, error) {
	normalizePRIntegerDependencies(deps)
	if err := validatePRIntegerEntry(entry); err != nil {
		return prIntegerEntryPaths{}, err
	}
	paths := prIntegerEntryPaths{
		safeImage:    strings.NewReplacer("/", "-", ":", "-").Replace(entry.Image),
		securityRoot: resolvePRIntegerPath(request.RepoRoot, request.SecurityDir),
	}
	paths.reportDir = filepath.Join(
		resolvePRIntegerPath(request.RepoRoot, request.ReportsDir),
		paths.safeImage+"-"+entry.Version+"-"+entry.Type,
	)
	if err := os.MkdirAll(paths.reportDir, 0o755); err != nil {
		return prIntegerEntryPaths{}, fmt.Errorf("create Integer report directory: %w", err)
	}
	fmt.Fprintf(deps.Stdout, "::group::Melange %s:%s-%s (%s)\n", entry.Image, entry.Version, entry.Type, request.PackageArchitecture)
	started := time.Now()
	buildErr := runPRMelangeBuild(ctx, deps, request, entry, false)
	fmt.Fprintln(deps.Stdout, "::endgroup::")
	if buildErr != nil {
		return prIntegerEntryPaths{}, fmt.Errorf("melange %s:%s-%s failed after %s: %w", entry.Image, entry.Version, entry.Type, time.Since(started).Round(time.Second), buildErr)
	}
	if err := runPRIntegerPackageChecks(ctx, deps, request, entry); err != nil {
		return prIntegerEntryPaths{}, err
	}
	if request.Kind == prIntegerBatchBuild && entry.Image == "linkerd" && entry.Version == "25" && entry.Type == "default" {
		if err := runPRLinkerdCanary(ctx, deps, request, entry); err != nil {
			return prIntegerEntryPaths{}, err
		}
	}
	return paths, nil
}

func resolvePRIntegerPath(root, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(root, path)
}
