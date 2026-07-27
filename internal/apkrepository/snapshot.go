package apkrepository

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

type SnapshotOptions = AssembleOptions

func Snapshot(ctx context.Context, options *SnapshotOptions) error {
	if options == nil {
		return errOptionsRequired
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("snapshot APK repository: %w", err)
	}
	config, err := parseAssembleOptions(options)
	if err != nil {
		return err
	}
	if len(config.privateKeyPEM) == 0 {
		return errSigningKeyRequired
	}
	packages, err := findPackages(config.sources)
	if err != nil {
		return err
	}
	if err := validateSnapshotPackages(packages); err != nil {
		return err
	}
	stage, err := prepareStagedOutput(config.outputDir)
	if err != nil {
		return err
	}
	defer stage.cleanup()
	stagedConfig := *config
	stagedConfig.outputDir = stage.path
	var output bytes.Buffer
	stagedConfig.stdout = &output
	if err := cleanManagedRepository(stage.path, config.keyName); err != nil {
		return err
	}
	if err := assemblePackages(ctx, &stagedConfig, packages); err != nil {
		return err
	}
	stagedPath := stage.path
	if err := stage.commit(); err != nil {
		return err
	}
	message := strings.ReplaceAll(output.String(), stagedPath, config.outputDir)
	if _, err := io.WriteString(config.stdout, message); err != nil {
		return fmt.Errorf("write snapshot result: %w", err)
	}
	return nil
}

func validateSnapshotPackages(paths []string) error {
	packages := make(map[packageKey]string, len(paths))
	counts := make(map[string]int)
	for _, path := range paths {
		metadata, err := inspectPackage(path)
		if err != nil {
			return err
		}
		parentArchitecture := filepath.Base(filepath.Dir(path))
		if metadata.key.architecture != parentArchitecture {
			return fmt.Errorf("%w: %s declares %s under %s", errUnsupportedPackageArchitecture, path, metadata.key.architecture, parentArchitecture)
		}
		if previous, exists := packages[metadata.key]; exists {
			return fmt.Errorf("%w %s/%s: %s and %s", errDuplicatePackageKey, metadata.key.architecture, metadata.key.name, previous, path)
		}
		packages[metadata.key] = path
		counts[metadata.key.architecture]++
	}
	for _, required := range []string{"x86_64", "aarch64"} {
		if counts[required] == 0 {
			return fmt.Errorf("%w: %s", errSnapshotIncomplete, required)
		}
	}
	return nil
}
