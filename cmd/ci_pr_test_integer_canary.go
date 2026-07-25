package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

var prLinkerdLocalPinPattern = regexp.MustCompile(`linkerd2-cli=[^ ,]+@local`)

func runPRLinkerdCanary(
	ctx context.Context,
	deps *prIntegerDependencies,
	request *prIntegerBatchRequest,
	entry prIntegerEntry,
) error {
	if err := runPRMelangeBuild(ctx, deps, request, entry, true); err != nil {
		return fmt.Errorf("staged Melange canary: %w", err)
	}
	genDir := filepath.Join(request.RepoRoot, "pin-config-gen")
	if _, err := runPRIntegerCommand(ctx, deps, &prCommandRequest{
		Name: request.VerityPath, Dir: request.RepoRoot,
		Args: []string{"integer", "discover", "--apkindex-url", "", "--gen-dir", genDir},
	}); err != nil {
		return fmt.Errorf("discover pin-config canary: %w", err)
	}
	config := filepath.Join(genDir, entry.Image, entry.Version, entry.Type+".apko.yaml")
	if _, err := runPRIntegerCommand(ctx, deps, &prCommandRequest{
		Name: request.VerityPath, Dir: request.RepoRoot,
		Args: []string{
			"integer", "melange", "pin-config", "--config", config,
			"--repository", "packages/repo", "--arch", request.PackageArchitecture,
		},
	}); err != nil {
		return fmt.Errorf("pin canary package: %w", err)
	}
	data, err := os.ReadFile(config)
	if err != nil || !prLinkerdLocalPinPattern.Match(data) {
		return fmt.Errorf("%w: Linkerd canary config is not pinned to @local", errPRCommandFailed)
	}
	imagePath := filepath.Join(request.RepoRoot, "pin-config-"+request.Architecture+".tar")
	reportPath := filepath.Join(request.RepoRoot, "pin-config-"+request.Architecture+".json")
	if _, err := runPRIntegerCommand(ctx, deps, &prCommandRequest{
		Name: "apko", Dir: request.RepoRoot,
		Args: []string{
			"build", "--arch", request.Architecture, "--repository-append", "@local packages/repo",
			"--keyring-append", "melange-work/melange.rsa.pub", config, "integer:pin-config-canary", imagePath,
		},
	}); err != nil {
		return fmt.Errorf("build pin-config canary: %w", err)
	}
	_, err = runPRIntegerTrivy(ctx, deps, request.RepoRoot, imagePath, reportPath)
	return err
}
