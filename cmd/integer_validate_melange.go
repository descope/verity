package cmd

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/verity-org/verity/internal/integer/apkindex"
	intconfig "github.com/verity-org/verity/internal/integer/config"
)

var (
	errMissingBespokePackageName = errors.New("missing package.name")
	errBespokePackageMismatch    = errors.New("package.name is not satisfiable by the image packages")
)

type versionedMelangeSpec struct {
	version string
	spec    *intconfig.MelangeSpec
}

func (s versionedMelangeSpec) resolveBespokeFiles(def *intconfig.ImageDef, typeName, bespokeFile string) ([]string, error) {
	if s.version != "" {
		return []string{strings.ReplaceAll(bespokeFile, "{{version}}", s.version)}, nil
	}
	return resolveBespokeFiles(def, typeName, bespokeFile)
}

func (s versionedMelangeSpec) packageMatcher(def *intconfig.ImageDef, typeName string, packages []string) func(string) bool {
	if s.version != "" {
		return func(pkgName string) bool {
			for _, pkg := range packages {
				if apkPackageName(strings.ReplaceAll(pkg, "{{version}}", s.version)) == pkgName {
					return true
				}
			}
			return false
		}
	}
	return func(pkgName string) bool {
		return tmplPackageMatchesBespoke(def, typeName, packages, pkgName)
	}
}

func melangeSpecsForType(def *intconfig.ImageDef, typeName string) []versionedMelangeSpec {
	selected := []versionedMelangeSpec{}
	if tmpl, ok := def.Types[typeName]; ok && tmpl.Melange != nil {
		selected = append(selected, versionedMelangeSpec{spec: tmpl.Melange})
	}
	versions := make([]string, 0, len(def.Versions))
	for version, meta := range def.Versions {
		if !slices.Contains(meta.SkipTypes, typeName) {
			versions = append(versions, version)
		}
	}
	apkindex.SortVersions(versions)
	for _, version := range versions {
		if spec, configured := def.Versions[version].Melange[typeName]; configured && spec != nil {
			selected = append(selected, versionedMelangeSpec{version: version, spec: spec})
		}
	}
	return selected
}

func resolveBespokeFiles(def *intconfig.ImageDef, typeName, bespokeFile string) ([]string, error) {
	if !strings.Contains(bespokeFile, "{{version}}") {
		return []string{bespokeFile}, nil
	}

	versions := make([]string, 0, len(def.Versions))
	for version, meta := range def.Versions {
		if slices.Contains(meta.SkipTypes, typeName) {
			continue
		}
		versions = append(versions, version)
	}
	if len(versions) == 0 {
		return nil, fmt.Errorf("%w: %q", errMissingDeclaredVersionType, typeName)
	}

	apkindex.SortVersions(versions)
	resolved := make([]string, 0, len(versions))
	seen := make(map[string]struct{}, len(versions))
	for _, version := range versions {
		name := strings.ReplaceAll(bespokeFile, "{{version}}", version)
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		resolved = append(resolved, name)
	}
	return resolved, nil
}

func validateBespokePackage(path string, matches func(string) bool) error {
	pkgName, err := readBespokePackageName(path)
	if err != nil {
		return err
	}
	if pkgName == "" {
		return errMissingBespokePackageName
	}
	if !matches(pkgName) {
		return fmt.Errorf("%w: %q", errBespokePackageMismatch, pkgName)
	}
	return nil
}
