package apkindex

import (
	"errors"
	"fmt"
	"strings"

	apkversion "github.com/knqyf263/go-apk-version"
)

var errInvalidPackageConstraint = errors.New("invalid APK package constraint")

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
