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

var safeKeyName = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

type AssembleOptions struct {
	OutputDir     string
	KeyName       string
	PublicKeyPath string
	Sources       []string
	PrivateKeyPEM []byte
	Stdout        io.Writer
	Stderr        io.Writer
	runner        commandRunner
}

func Assemble(ctx context.Context, options *AssembleOptions) error {
	if options == nil {
		return errOptionsRequired
	}
	defer clear(options.PrivateKeyPEM)
	config, err := parseAssembleOptions(options)
	if err != nil {
		return err
	}
	packages, err := findPackages(config.sources)
	if err != nil {
		return err
	}
	if err := cleanManagedRepository(config.outputDir, config.keyName); err != nil {
		return err
	}
	if len(packages) == 0 {
		marker := fmt.Sprintf("No APK files were found in: %s\nRepository assembly skipped without failing the workflow.\n", strings.Join(config.sources, " "))
		if err := os.WriteFile(filepath.Join(config.outputDir, ".no-apks-found"), []byte(marker), 0o644); err != nil {
			return fmt.Errorf("write empty repository marker: %w", err)
		}
		_, err := fmt.Fprintf(config.stdout, "No APK files found; wrote %s\n", filepath.Join(config.outputDir, ".no-apks-found"))
		return err
	}
	return assemblePackages(ctx, config, packages)
}

type assembleConfig struct {
	outputDir     string
	keyName       string
	publicKeyPath string
	sources       []string
	privateKeyPEM []byte
	stdout        io.Writer
	stderr        io.Writer
	runner        commandRunner
}

func parseAssembleOptions(options *AssembleOptions) (*assembleConfig, error) {
	outputDir := options.OutputDir
	if outputDir == "" {
		outputDir = "site/dist/apk"
	}
	if err := validateOutputDirectory(outputDir); err != nil {
		return nil, err
	}
	keyName := options.KeyName
	if keyName == "" {
		keyName = "verity.rsa"
	}
	if !safeKeyName.MatchString(keyName) || strings.Contains(keyName, "..") {
		return nil, fmt.Errorf("%w: %s", errUnsafeKeyName, keyName)
	}
	if !strings.HasSuffix(keyName, ".rsa") {
		return nil, fmt.Errorf("%w so signatures match the published .rsa.pub key: %s", errRSAKeyNameRequired, keyName)
	}
	publicKeyPath := options.PublicKeyPath
	if publicKeyPath == "" {
		publicKeyPath = filepath.Join("keys", "apk", keyName+".pub")
	}
	sources := options.Sources
	if len(sources) == 0 {
		sources = []string{"packages/repo", "apk-artifacts"}
	}
	runner := options.runner
	if runner == nil {
		runner = execCommandRunner{}
	}
	return &assembleConfig{
		outputDir: outputDir, keyName: keyName, publicKeyPath: publicKeyPath,
		sources: sources, privateKeyPEM: options.PrivateKeyPEM,
		stdout: writerOrDiscard(options.Stdout), stderr: writerOrDiscard(options.Stderr), runner: runner,
	}, nil
}

func findPackages(sources []string) ([]string, error) {
	unique := make(map[string]struct{})
	for _, source := range sources {
		info, err := os.Stat(source)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("stat source %q: %w", source, err)
		}
		if !info.IsDir() {
			continue
		}
		err = filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.Type().IsRegular() && strings.HasSuffix(entry.Name(), ".apk") {
				unique[path] = struct{}{}
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("scan source %q: %w", source, err)
		}
	}
	packages := make([]string, 0, len(unique))
	for path := range unique {
		packages = append(packages, path)
	}
	sort.Strings(packages)
	return packages, nil
}

func cleanManagedRepository(outputDir, keyName string) error {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	for _, name := range []string{".no-apks-found", keyName + ".pub", "repository-format"} {
		if err := removeIfExists(filepath.Join(outputDir, name)); err != nil {
			return err
		}
	}
	for _, architecture := range supportedArches {
		if err := os.RemoveAll(filepath.Join(outputDir, architecture)); err != nil {
			return fmt.Errorf("remove managed architecture %s: %w", architecture, err)
		}
	}
	return nil
}
