package apkrepository

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type repositoryPackageSet map[packageKey][]inspectedPackage

type deltaBase struct {
	packages repositoryPackageSet
	keyPath  string
}

func loadDeltaBase(config *deltaConfig, manifest *DeltaManifest) (*deltaBase, error) {
	digest, err := RepositoryDigest(config.baseDir)
	if err != nil {
		return nil, err
	}
	if digest != manifest.BaseSHA256 {
		return nil, fmt.Errorf("%w: manifest has %s, repository has %s", errDeltaBaseMismatch, manifest.BaseSHA256, digest)
	}
	formatPath := filepath.Join(config.baseDir, "repository-format")
	formatBytes, err := os.ReadFile(formatPath)
	if err != nil {
		return nil, fmt.Errorf("read repository format: %w", err)
	}
	format := strings.TrimSpace(string(formatBytes))
	if format != repositoryFormatVersion || manifest.RepositoryFormat != format {
		return nil, fmt.Errorf("%w: manifest=%q base=%q supported=%q", errDeltaFormatMismatch, manifest.RepositoryFormat, format, repositoryFormatVersion)
	}
	keyPath := filepath.Join(config.baseDir, config.keyName+".pub")
	keyDigest, err := fileSHA256(keyPath)
	if err != nil {
		return nil, err
	}
	if keyDigest != manifest.KeySHA256 {
		return nil, fmt.Errorf("%w: manifest has %s, repository has %s", errDeltaKeyMismatch, manifest.KeySHA256, keyDigest)
	}
	packages, err := inspectRepositoryPackages(config.baseDir)
	if err != nil {
		return nil, err
	}
	if err := requireDualArchitecturePackages(packages); err != nil {
		return nil, err
	}
	return &deltaBase{packages: packages, keyPath: keyPath}, nil
}

func inspectRepositoryPackages(repository string) (repositoryPackageSet, error) {
	packages := make(repositoryPackageSet)
	for _, architecture := range supportedArches {
		paths, err := filepath.Glob(filepath.Join(repository, architecture, "*.apk"))
		if err != nil {
			return nil, fmt.Errorf("list %s packages: %w", architecture, err)
		}
		sort.Strings(paths)
		for _, path := range paths {
			metadata, err := inspectPackage(path)
			if err != nil {
				return nil, err
			}
			if metadata.key.architecture != architecture {
				return nil, fmt.Errorf("%w: %s declares %s under %s", errDeltaPackageMismatch, path, metadata.key.architecture, architecture)
			}
			packages[metadata.key] = append(packages[metadata.key], metadata)
		}
	}
	return packages, nil
}

func requireDualArchitecturePackages(packages repositoryPackageSet) error {
	counts := make(map[string]int)
	for key, versions := range packages {
		counts[key.architecture] += len(versions)
	}
	for _, required := range []string{"x86_64", "aarch64"} {
		if counts[required] == 0 {
			return fmt.Errorf("%w: %s", errSnapshotIncomplete, required)
		}
	}
	return nil
}
