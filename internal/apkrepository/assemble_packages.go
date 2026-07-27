package apkrepository

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func assemblePackages(ctx context.Context, config *assembleConfig, packages []string) (resultErr error) {
	temporaryDir, err := os.MkdirTemp("", "verity-apk-repository-")
	if err != nil {
		return fmt.Errorf("create temporary directory: %w", err)
	}
	defer func() {
		if err := os.RemoveAll(temporaryDir); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("remove signing directory: %w", err))
		}
	}()
	privateKey, _, err := configureSigning(config, temporaryDir)
	if err != nil {
		return err
	}
	destinations := make(map[string]string)
	for _, packagePath := range packages {
		architecture := filepath.Base(filepath.Dir(packagePath))
		if !isSupportedArchitecture(architecture) {
			return fmt.Errorf("%w from parent directory: %s", errUnsupportedPackageArchitecture, packagePath)
		}
		destinationKey := filepath.Join(architecture, filepath.Base(packagePath))
		candidate := filepath.Join(temporaryDir, "packages", destinationKey)
		if err := copyFile(packagePath, candidate); err != nil {
			return err
		}
		if privateKey != "" {
			if _, err := runRequired(ctx, config.runner, &command{name: "melange", args: []string{"sign", "--signing-key", privateKey, candidate}, sensitive: true}); err != nil {
				return fmt.Errorf("sign package %s: %w", packagePath, err)
			}
		}
		destination := filepath.Join(config.outputDir, destinationKey)
		if previousSource, exists := destinations[destinationKey]; exists {
			same, err := filesEqual(candidate, destination)
			if err != nil {
				return err
			}
			if !same {
				return fmt.Errorf("%w %s: %s and %s", errDuplicateDestination, filepath.ToSlash(destinationKey), previousSource, packagePath)
			}
			fmt.Fprintf(config.stdout, "Skipped byte-identical duplicate APK: %s\n", filepath.ToSlash(destinationKey))
			continue
		}
		if err := copyFile(candidate, destination); err != nil {
			return err
		}
		destinations[destinationKey] = packagePath
	}
	return createIndexes(ctx, config, privateKey)
}

func configureSigning(config *assembleConfig, temporaryDir string) (privateKey, publicKey string, err error) {
	publicKey = filepath.Join(config.outputDir, config.keyName+".pub")
	if len(config.privateKeyPEM) == 0 {
		fmt.Fprintln(config.stderr, "APK signing key was not provided on stdin; APKINDEX files will be unsigned")
		return "", publicKey, nil
	}
	privateKey = filepath.Join(temporaryDir, config.keyName)
	if err := prepareSigningKey(config.privateKeyPEM, config.publicKeyPath, privateKey); err != nil {
		return "", "", err
	}
	if err := copyFile(config.publicKeyPath, publicKey); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(filepath.Join(config.outputDir, "repository-format"), []byte(repositoryFormatVersion+"\n"), 0o644); err != nil {
		return "", "", fmt.Errorf("write repository format: %w", err)
	}
	fmt.Fprintf(config.stdout, "Published APK repository public key: %s\n", publicKey)
	return privateKey, publicKey, nil
}

func filesEqual(first, second string) (bool, error) {
	firstBytes, err := os.ReadFile(first)
	if err != nil {
		return false, fmt.Errorf("read %q: %w", first, err)
	}
	secondBytes, err := os.ReadFile(second)
	if err != nil {
		return false, fmt.Errorf("read %q: %w", second, err)
	}
	return bytes.Equal(firstBytes, secondBytes), nil
}

func createIndexes(ctx context.Context, config *assembleConfig, privateKey string) error {
	return createIndexesForArchitectures(ctx, config, privateKey, supportedArches)
}

func createIndexesForArchitectures(ctx context.Context, config *assembleConfig, privateKey string, architectures []string) error {
	for _, architecture := range architectures {
		architectureDir := filepath.Join(config.outputDir, architecture)
		packages, err := filepath.Glob(filepath.Join(architectureDir, "*.apk"))
		if err != nil {
			return fmt.Errorf("list packages for %s: %w", architecture, err)
		}
		if len(packages) == 0 {
			if err := removeIfExists(filepath.Join(architectureDir, "APKINDEX.tar.gz")); err != nil {
				return err
			}
			continue
		}
		args := []string{"index", "--arch", architecture, "--output", "APKINDEX.tar.gz"}
		if privateKey != "" {
			args = append(args, "--signing-key", privateKey)
		}
		for _, packagePath := range packages {
			args = append(args, "./"+filepath.Base(packagePath))
		}
		if _, err := runRequired(ctx, config.runner, &command{name: "melange", args: args, dir: architectureDir, sensitive: privateKey != ""}); err != nil {
			return fmt.Errorf("build index for %s: %w", architecture, err)
		}
		fmt.Fprintf(config.stdout, "Assembled %s/APKINDEX.tar.gz (%d packages)\n", architecture, len(packages))
	}
	return nil
}
