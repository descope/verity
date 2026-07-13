package apkindex

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	apkversion "github.com/knqyf263/go-apk-version"
)

var (
	errAmbiguousPackageProvider  = errors.New("multiple local packages provide dependency")
	errInvalidPackageConstraint  = errors.New("invalid APK package constraint")
	errUnsatisfiedPackageVersion = errors.New("local package does not satisfy dependency constraint")
)

func PackageName(packageSpec string) string {
	if index := strings.IndexAny(packageSpec, "@<>=~!"); index >= 0 {
		return packageSpec[:index]
	}
	return packageSpec
}

func PackageSatisfiesConstraint(packageSpec, candidateVersion string) (bool, error) {
	operator, requiredVersion, constrained, err := packageConstraint(packageSpec)
	if err != nil {
		return false, err
	}
	if !constrained {
		return true, nil
	}

	candidate, err := apkversion.NewVersion(candidateVersion)
	if err != nil {
		return false, fmt.Errorf("parse candidate version %q: %w", candidateVersion, err)
	}
	required, err := apkversion.NewVersion(requiredVersion)
	if err != nil {
		return false, fmt.Errorf("parse required version %q: %w", requiredVersion, err)
	}
	comparison := candidate.Compare(required)
	if strings.Contains(operator, "~") && fuzzyVersionEqual(candidateVersion, requiredVersion, comparison) {
		comparison = 0
	}
	return constraintMatches(operator, comparison, packageSpec)
}

func ResolveDependency(packages map[string]Package, dependency string) (Package, bool, error) {
	name := PackageName(dependency)
	if name == "" {
		return Package{}, false, nil
	}
	var concreteError error
	if pkg, exists := packages[name]; exists {
		satisfied, err := PackageSatisfiesConstraint(dependency, pkg.Version)
		if err != nil {
			return Package{}, false, err
		}
		if satisfied {
			return pkg, true, nil
		}
		concreteError = fmt.Errorf("%w: %s requires %s, local version is %s", errUnsatisfiedPackageVersion, name, dependency, pkg.Version)
	}
	pkg, local, err := resolveProvidedDependency(packages, dependency, name)
	if err != nil {
		return Package{}, false, err
	}
	if local {
		return pkg, true, nil
	}
	return Package{}, false, concreteError
}

func resolveProvidedDependency(packages map[string]Package, dependency, name string) (Package, bool, error) {
	matches := map[string]Package{}
	for _, pkg := range packages {
		for _, provided := range pkg.Provides {
			if PackageName(provided) != name {
				continue
			}
			satisfied, err := providedDependencySatisfies(dependency, provided)
			if err != nil {
				return Package{}, false, err
			}
			if satisfied {
				matches[pkg.Name] = pkg
			}
		}
	}
	if len(matches) == 0 {
		return Package{}, false, nil
	}
	if len(matches) == 1 {
		for _, pkg := range matches {
			return pkg, true, nil
		}
	}
	names := make([]string, 0, len(matches))
	for packageName := range matches {
		names = append(names, packageName)
	}
	sort.Strings(names)
	return Package{}, false, fmt.Errorf("%w %q: %s", errAmbiguousPackageProvider, dependency, strings.Join(names, ", "))
}

func providedDependencySatisfies(dependency, provided string) (bool, error) {
	_, _, dependencyConstrained, err := packageConstraint(dependency)
	if err != nil {
		return false, err
	}
	operator, version, providerConstrained, err := packageConstraint(provided)
	if err != nil {
		return false, err
	}
	if providerConstrained && operator != "=" {
		return false, fmt.Errorf("%w %q", errInvalidPackageConstraint, provided)
	}
	if !dependencyConstrained {
		return true, nil
	}
	if !providerConstrained {
		return false, nil
	}
	return PackageSatisfiesConstraint(dependency, version)
}

func packageConstraint(packageSpec string) (operator, version string, constrained bool, err error) {
	untagged, _, _ := strings.Cut(packageSpec, "@")
	name := PackageName(untagged)
	if name == "" {
		return "", "", false, fmt.Errorf("%w %q", errInvalidPackageConstraint, packageSpec)
	}
	remainder := strings.TrimPrefix(untagged, name)
	if remainder == "" {
		return "", "", false, nil
	}
	operatorLength := 0
	for operatorLength < len(remainder) && strings.ContainsRune("<>=~", rune(remainder[operatorLength])) {
		operatorLength++
	}
	if operatorLength == 0 || operatorLength == len(remainder) {
		return "", "", false, fmt.Errorf("%w %q", errInvalidPackageConstraint, packageSpec)
	}
	return remainder[:operatorLength], remainder[operatorLength:], true, nil
}

func constraintMatches(operator string, comparison int, packageSpec string) (bool, error) {
	switch operator {
	case "=":
		return comparison == 0, nil
	case "<":
		return comparison < 0, nil
	case "<=", "<~":
		return comparison <= 0, nil
	case "~":
		return comparison == 0, nil
	case ">":
		return comparison > 0, nil
	case ">=", ">~":
		return comparison >= 0, nil
	default:
		return false, fmt.Errorf("%w %q", errInvalidPackageConstraint, packageSpec)
	}
}

func fuzzyVersionEqual(candidate, required string, comparison int) bool {
	if comparison == 0 {
		return true
	}
	if !strings.HasPrefix(candidate, required) || len(candidate) == len(required) {
		return false
	}
	next := candidate[len(required)]
	return next < '0' || next > '9'
}
