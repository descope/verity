package melange

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/verity-org/verity/internal/integer/apkindex"
)

const localRepositoryTag = "local"

var (
	errPinnedConfigNotRegular             = errors.New("apko config is not a regular file")
	errPinnedConfigPackageNotScalar       = errors.New("apko config package entry is not a scalar")
	errPinnedConfigPackagesMissing        = errors.New("apko config contents.packages is missing")
	errPinnedDependencyConstraint         = errors.New("local package dependency constraint is not satisfied")
	errPinnedIndexNotRegular              = errors.New("local package index is not a regular file")
	errPinnedPackageMissingArch           = errors.New("local package is missing for an architecture")
	errPinnedPackageNotUsed               = errors.New("apko config does not use a local package")
	errPinnedPackageVersionConflict       = errors.New("local package versions conflict")
	errPinnedPackageVersionUndefined      = errors.New("local package version is empty")
	errPinnedProviderArchitectureConflict = errors.New("local dependency resolves to different packages across architectures")
)

type PinConfigOptions struct {
	RootDir       string
	ConfigPath    string
	RepositoryDir string
	Architectures []Architecture
}

func PinConfigPackages(options PinConfigOptions) error {
	rootDir, repositoryRelative, configRelative, err := pinConfigPaths(options)
	if err != nil {
		return err
	}
	packageSets, err := loadPinnedPackages(rootDir, repositoryRelative, options.Architectures)
	if err != nil {
		return err
	}
	configInfo, err := os.Lstat(options.ConfigPath)
	if err != nil {
		return fmt.Errorf("inspect apko config %q: %w", options.ConfigPath, err)
	}
	if !configInfo.Mode().IsRegular() {
		return fmt.Errorf("%w: %s", errPinnedConfigNotRegular, options.ConfigPath)
	}
	data, err := readRegularFile(rootDir, configRelative)
	if err != nil {
		return fmt.Errorf("read apko config %q: %w", options.ConfigPath, pinnedRegularFileError(err, errPinnedConfigNotRegular))
	}
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return fmt.Errorf("parse apko config %q: %w", options.ConfigPath, err)
	}
	packages, err := configPackagesNode(&document)
	if err != nil {
		return fmt.Errorf("parse apko config %q: %w", options.ConfigPath, err)
	}

	resolver := pinnedPackageResolver{
		architectures: options.Architectures,
		packageSets:   packageSets,
	}
	queue, pinned, err := resolver.pinConfiguredPackages(options.ConfigPath, packages)
	if err != nil {
		return err
	}
	if len(queue) == 0 {
		return errPinnedPackageNotUsed
	}
	if err := resolver.appendPinnedDependencies(packages, queue, pinned); err != nil {
		return err
	}

	output, err := yaml.Marshal(&document)
	if err != nil {
		return fmt.Errorf("encode apko config %q: %w", options.ConfigPath, err)
	}
	if err := replaceRegularFile(rootDir, options.ConfigPath, output, configInfo.Mode().Perm(), errPinnedConfigNotRegular); err != nil {
		return err
	}
	return nil
}

func pinConfigPaths(options PinConfigOptions) (rootDir, repositoryRelative, configRelative string, err error) {
	rootDir = options.RootDir
	if rootDir == "" {
		rootDir = "."
	}
	repositoryRelative, err = relativeToRoot(rootDir, options.RepositoryDir)
	if err != nil {
		return "", "", "", fmt.Errorf("locate local package repository %q: %w", options.RepositoryDir, err)
	}
	configRelative, err = relativeToRoot(rootDir, options.ConfigPath)
	if err != nil {
		return "", "", "", fmt.Errorf("locate apko config %q: %w", options.ConfigPath, err)
	}
	return rootDir, repositoryRelative, configRelative, nil
}

func loadPinnedPackages(rootDir, repositoryRelative string, architectures []Architecture) (pinnedPackageSets, error) {
	packageSets := make(pinnedPackageSets, len(architectures))
	for _, architecture := range architectures {
		if !architecture.valid() {
			return nil, fmt.Errorf("%w %q", errUnsupportedArchitecture, architecture)
		}
		indexRelative := filepath.Join(repositoryRelative, string(architecture), "APKINDEX.tar.gz")
		indexPath := filepath.Join(rootDir, indexRelative)
		index, err := readRegularFile(rootDir, indexRelative)
		if err != nil {
			return nil, fmt.Errorf("read local package index %q: %w", indexPath, pinnedRegularFileError(err, errPinnedIndexNotRegular))
		}
		packages, err := apkindex.ParseArchive(bytes.NewReader(index))
		if err != nil {
			return nil, fmt.Errorf("parse local package index %q: %w", indexPath, err)
		}
		architecturePackages := make(map[string]apkindex.Package, len(packages))
		for _, pkg := range packages {
			if pkg.Version == "" {
				return nil, fmt.Errorf("%w: %s in %s", errPinnedPackageVersionUndefined, pkg.Name, architecture)
			}
			if previous, exists := architecturePackages[pkg.Name]; exists && previous.Version != pkg.Version {
				return nil, fmt.Errorf("%w for %s in %s: %s and %s", errPinnedPackageVersionConflict, pkg.Name, architecture, previous.Version, pkg.Version)
			}
			architecturePackages[pkg.Name] = pkg
		}
		packageSets[architecture] = architecturePackages
	}
	return packageSets, nil
}

func pinnedRegularFileError(err, notRegular error) error {
	if errors.Is(err, errNotRegularFile) || errors.Is(err, errPathContainsSymlink) || errors.Is(err, errNotRealDirectory) {
		return notRegular
	}
	return err
}

func configPackagesNode(document *yaml.Node) (*yaml.Node, error) {
	if document.Kind != yaml.DocumentNode || len(document.Content) != 1 {
		return nil, errPinnedConfigPackagesMissing
	}
	contents := yamlMappingValue(document.Content[0], "contents")
	if contents == nil {
		return nil, errPinnedConfigPackagesMissing
	}
	packages := yamlMappingValue(contents, "packages")
	if packages == nil || packages.Kind != yaml.SequenceNode {
		return nil, errPinnedConfigPackagesMissing
	}
	return packages, nil
}

func yamlMappingValue(mapping *yaml.Node, key string) *yaml.Node {
	if mapping.Kind != yaml.MappingNode {
		return nil
	}
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value == key {
			return mapping.Content[index+1]
		}
	}
	return nil
}
