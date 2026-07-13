package melange

import (
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/verity-org/verity/internal/integer/apkindex"
)

type pinnedPackageSets map[Architecture]map[string]apkindex.Package

type pinnedPackageResolver struct {
	architectures []Architecture
	packageSets   pinnedPackageSets
}

func (resolver pinnedPackageResolver) packageVersion(name string) (version string, local bool, err error) {
	var selected string
	found := false
	for _, architecture := range resolver.architectures {
		pkg, exists := resolver.packageSets[architecture][name]
		if !exists {
			continue
		}
		if found && pkg.Version != selected {
			return "", false, fmt.Errorf("%w for %s: %s and %s", errPinnedPackageVersionConflict, name, selected, pkg.Version)
		}
		selected = pkg.Version
		found = true
	}
	if !found {
		return "", false, nil
	}
	for _, architecture := range resolver.architectures {
		if _, exists := resolver.packageSets[architecture][name]; !exists {
			return "", false, fmt.Errorf("%w: %s for %s", errPinnedPackageMissingArch, name, architecture)
		}
	}
	return selected, true, nil
}

func (resolver pinnedPackageResolver) pinConfiguredPackages(configPath string, packages *yaml.Node) (queue []string, pinned map[string]struct{}, err error) {
	queue = make([]string, 0, len(packages.Content))
	pinned = make(map[string]struct{}, len(packages.Content))
	for _, packageNode := range packages.Content {
		if packageNode.Kind != yaml.ScalarNode {
			return nil, nil, fmt.Errorf("parse apko config %q: %w", configPath, errPinnedConfigPackageNotScalar)
		}
		name := apkindex.PackageName(packageNode.Value)
		version, local, err := resolver.packageVersion(name)
		if err != nil {
			return nil, nil, err
		}
		if !local {
			continue
		}
		packageNode.Value = name + "=" + version + "@" + localRepositoryTag
		if _, exists := pinned[name]; !exists {
			pinned[name] = struct{}{}
			queue = append(queue, name)
		}
	}
	return queue, pinned, nil
}

func (resolver pinnedPackageResolver) appendPinnedDependencies(packages *yaml.Node, queue []string, pinned map[string]struct{}) error {
	for cursor := 0; cursor < len(queue); cursor++ {
		dependencies, err := resolver.localDependencies(queue[cursor])
		if err != nil {
			return err
		}
		for _, name := range dependencies {
			if _, exists := pinned[name]; exists {
				continue
			}
			version, local, err := resolver.packageVersion(name)
			if err != nil {
				return err
			}
			if !local {
				continue
			}
			pinned[name] = struct{}{}
			queue = append(queue, name)
			packages.Content = append(packages.Content, &yaml.Node{
				Kind:  yaml.ScalarNode,
				Tag:   "!!str",
				Value: name + "=" + version + "@" + localRepositoryTag,
			})
		}
	}
	return nil
}

func (resolver pinnedPackageResolver) localDependencies(name string) ([]string, error) {
	dependencies := []string{}
	providers := map[string]string{}
	seen := map[string]struct{}{}
	for _, architecture := range resolver.architectures {
		pkg, exists := resolver.packageSets[architecture][name]
		if !exists {
			continue
		}
		for _, dependency := range pkg.Dependencies {
			localPackage, local, err := apkindex.ResolveDependencyForPackage(resolver.packageSets[architecture], &pkg, dependency)
			if err != nil {
				return nil, fmt.Errorf("%w: %s dependency %q for %s: %w", errPinnedDependencyConstraint, name, dependency, architecture, err)
			}
			if !local {
				continue
			}
			dependencyName := localPackage.Name
			providedName := apkindex.PackageName(dependency)
			if previous, exists := providers[providedName]; exists && previous != dependencyName {
				return nil, fmt.Errorf("%w: %s resolves %s to %s for %s and %s for another architecture", errPinnedProviderArchitectureConflict, name, providedName, dependencyName, architecture, previous)
			}
			providers[providedName] = dependencyName
			if _, exists := seen[dependencyName]; exists {
				continue
			}
			seen[dependencyName] = struct{}{}
			dependencies = append(dependencies, dependencyName)
		}
	}
	return dependencies, nil
}
