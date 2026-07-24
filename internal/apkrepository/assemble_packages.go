package apkrepository

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
)

func assemblePackages(ctx context.Context, config *assembleConfig, packages []string) error {
	temporaryDir, err := os.MkdirTemp("", "verity-apk-repository-")
	if err != nil {
		return fmt.Errorf("create temporary directory: %w", err)
	}
	defer os.RemoveAll(temporaryDir)
	privateKey, publicKey, err := configureSigning(config, temporaryDir)
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
			if _, err := runRequired(ctx, config.runner, command{name: "melange", args: []string{"sign", "--signing-key", privateKey, candidate}}); err != nil {
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
	return createIndexes(ctx, config, privateKey, publicKey)
}

func configureSigning(config *assembleConfig, temporaryDir string) (privateKey, publicKey string, err error) {
	publicKey = filepath.Join(config.outputDir, config.keyName+".pub")
	if len(config.privateKeyPEM) == 0 {
		fmt.Fprintln(config.stderr, "APK_REPOSITORY_PRIVATE_KEY not set; APKINDEX files will be unsigned")
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

func createIndexes(ctx context.Context, config *assembleConfig, privateKey, publicKey string) error {
	repositoryRoot, err := filepath.Abs(config.outputDir)
	if err != nil {
		return fmt.Errorf("resolve repository root: %w", err)
	}
	for _, architecture := range supportedArches {
		architectureDir := filepath.Join(config.outputDir, architecture)
		packages, err := filepath.Glob(filepath.Join(architectureDir, "*.apk"))
		if err != nil || len(packages) == 0 {
			continue
		}
		args := []string{"index"}
		if privateKey == "" {
			args = append(args, "--allow-untrusted")
		} else {
			args = append(args, "--keys-dir", repositoryRoot)
		}
		args = append(args, "--output", "APKINDEX.tar.gz")
		for _, packagePath := range packages {
			args = append(args, "./"+filepath.Base(packagePath))
		}
		if _, err := runRequired(ctx, config.runner, command{name: "apk", args: args, dir: architectureDir}); err != nil {
			return fmt.Errorf("build index for %s: %w", architecture, err)
		}
		indexPath := filepath.Join(architectureDir, "APKINDEX.tar.gz")
		if privateKey != "" {
			if _, err := runRequired(ctx, config.runner, command{name: "abuild-sign", args: []string{"-t", "RSA256", "-k", privateKey, "-p", publicKey, indexPath}}); err != nil {
				return fmt.Errorf("sign index for %s: %w", architecture, err)
			}
		}
		fmt.Fprintf(config.stdout, "Assembled %s/APKINDEX.tar.gz (%d packages)\n", architecture, len(packages))
	}
	return nil
}
