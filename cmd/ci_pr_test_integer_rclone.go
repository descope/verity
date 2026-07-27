package cmd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func runPRRcloneSmoke(
	ctx context.Context,
	deps *prIntegerDependencies,
	request *prIntegerBatchRequest,
	image string,
	metadata prPackageMetadata,
) error {
	if err := verifyPRRcloneVersion(ctx, deps, image, metadata.Version); err != nil {
		return err
	}
	if err := runPRRcloneCopySmoke(ctx, deps, request.RunnerTemp, image); err != nil {
		return err
	}
	return verifyPRRcloneSBOM(ctx, deps, request, image, metadata.FullVersion)
}

func verifyPRRcloneVersion(
	ctx context.Context,
	deps *prIntegerDependencies,
	image, version string,
) error {
	result, err := runPRIntegerCommand(ctx, deps, &prCommandRequest{
		Name: "docker", Args: []string{"run", "--rm", image, "--version"},
	})
	if err != nil {
		return fmt.Errorf("run rclone version: %w", err)
	}
	pattern := regexp.MustCompile(`^rclone v?` + regexp.QuoteMeta(version) + `$`)
	if !pattern.MatchString(strings.TrimSpace(string(result.Stdout))) {
		return fmt.Errorf("%w: unexpected rclone version output %q", errPRCommandFailed, strings.TrimSpace(string(result.Stdout)))
	}
	return nil
}

func runPRRcloneCopySmoke(
	ctx context.Context,
	deps *prIntegerDependencies,
	tempDir, image string,
) (err error) {
	smokeDir, err := os.MkdirTemp(tempDir, "verity-rclone-smoke-")
	if err != nil {
		return fmt.Errorf("create rclone smoke directory: %w", err)
	}
	defer func() {
		err = joinPRCleanup(err, func() error {
			if removeErr := os.RemoveAll(smokeDir); removeErr != nil {
				return fmt.Errorf("remove rclone smoke directory: %w", removeErr)
			}
			return nil
		})
	}()
	source := filepath.Join(smokeDir, "source")
	destination := filepath.Join(smokeDir, "destination")
	if err := os.MkdirAll(source, 0o755); err != nil {
		return fmt.Errorf("create rclone source: %w", err)
	}
	if err := os.MkdirAll(destination, 0o777); err != nil {
		return fmt.Errorf("create rclone destination: %w", err)
	}
	if err := os.Chmod(destination, 0o777); err != nil {
		return fmt.Errorf("make rclone destination writable: %w", err)
	}
	payload := []byte("verity rclone PR checksum smoke\n")
	if err := os.WriteFile(filepath.Join(source, "payload.txt"), payload, 0o644); err != nil {
		return fmt.Errorf("write rclone payload: %w", err)
	}
	volume := smokeDir + ":/work"
	for _, args := range [][]string{
		{"run", "--rm", "--volume", volume, image, "copy", "/work/source", "/work/destination"},
		{"run", "--rm", "--volume", volume, image, "check", "--checksum", "/work/source", "/work/destination"},
	} {
		if _, err := runPRIntegerCommand(ctx, deps, &prCommandRequest{Name: "docker", Args: args}); err != nil {
			return fmt.Errorf("run rclone checksum smoke: %w", err)
		}
	}
	copied, err := os.ReadFile(filepath.Join(destination, "payload.txt"))
	if err != nil {
		return fmt.Errorf("read copied rclone payload: %w", err)
	}
	if !bytes.Equal(copied, payload) {
		return fmt.Errorf("%w: rclone copied payload checksum mismatch", errPRCommandFailed)
	}
	return nil
}

func verifyPRRcloneSBOM(
	ctx context.Context,
	deps *prIntegerDependencies,
	request *prIntegerBatchRequest,
	image, fullVersion string,
) (err error) {
	container := fmt.Sprintf("integer-rclone-%s-pr-%s", request.Architecture, request.Kind)
	if _, err := runPRIntegerCommand(ctx, deps, &prCommandRequest{
		Name: "docker", Args: []string{"create", "--name", container, image, "--version"},
	}); err != nil {
		return fmt.Errorf("create rclone SBOM container: %w", err)
	}
	defer func() {
		err = joinPRCleanup(err, func() error { return removePRIntegerContainer(ctx, deps, container) })
	}()
	sbom := filepath.Join(request.RunnerTemp, fmt.Sprintf("rclone-%s-%s.spdx.json", request.Architecture, fullVersion))
	defer func() {
		err = joinPRCleanup(err, func() error {
			if removeErr := os.Remove(sbom); removeErr != nil && !os.IsNotExist(removeErr) {
				return fmt.Errorf("remove rclone SBOM: %w", removeErr)
			}
			return nil
		})
	}()
	if _, err := runPRIntegerCommand(ctx, deps, &prCommandRequest{
		Name: "docker",
		Args: []string{
			"cp", container + ":/var/lib/db/sbom/rclone-" + fullVersion + ".spdx.json", sbom,
		},
	}); err != nil {
		return fmt.Errorf("copy rclone SBOM: %w", err)
	}
	return verifyPRSPDXPackage(sbom, "rclone", fullVersion, "MIT")
}
