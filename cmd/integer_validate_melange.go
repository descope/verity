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
	version  string
	versions []string
	spec     *intconfig.MelangeSpec
}

func (s versionedMelangeSpec) resolveBespokeFiles(bespokeFile string) ([]string, error) {
	if s.version != "" {
		return []string{strings.ReplaceAll(bespokeFile, "{{version}}", s.version)}, nil
	}
	if !strings.Contains(bespokeFile, "{{version}}") {
		return []string{bespokeFile}, nil
	}
	if len(s.versions) == 0 {
		return nil, errMissingDeclaredVersionType
	}
	resolved := make([]string, 0, len(s.versions))
	seen := make(map[string]struct{}, len(s.versions))
	for _, version := range s.versions {
		name := strings.ReplaceAll(bespokeFile, "{{version}}", version)
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		resolved = append(resolved, name)
	}
	return resolved, nil
}

func (s versionedMelangeSpec) packageMatcher(packages []string) func(string) bool {
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
		for _, pkg := range packages {
			if apkPackageName(pkg) == pkgName {
				return true
			}
			if !strings.Contains(pkg, "{{version}}") {
				continue
			}
			for _, version := range s.versions {
				if apkPackageName(strings.ReplaceAll(pkg, "{{version}}", version)) == pkgName {
					return true
				}
			}
		}
		return false
	}
}

func melangeSpecsForType(def *intconfig.ImageDef, typeName string) []versionedMelangeSpec {
	selected := []versionedMelangeSpec{}
	versions := make([]string, 0, len(def.Versions))
	for version, meta := range def.Versions {
		if !slices.Contains(meta.SkipTypes, typeName) {
			versions = append(versions, version)
		}
	}
	apkindex.SortVersions(versions)
	if tmpl, ok := def.Types[typeName]; ok && tmpl.Melange != nil {
		sharedVersions := make([]string, 0, len(versions))
		for _, version := range versions {
			if def.MelangeFor(version, typeName) == tmpl.Melange {
				sharedVersions = append(sharedVersions, version)
			}
		}
		if len(def.Versions) == 0 || len(sharedVersions) > 0 {
			selected = append(selected, versionedMelangeSpec{versions: sharedVersions, spec: tmpl.Melange})
		}
	}
	for _, version := range versions {
		if spec, configured := def.Versions[version].Melange[typeName]; configured && spec != nil {
			selected = append(selected, versionedMelangeSpec{version: version, spec: spec})
		}
	}
	return selected
}

func validateBespokePackage(path string, packages []string, matches func(string) bool) error {
	pkgName, err := readBespokePackageName(path)
	if err != nil {
		return err
	}
	if pkgName == "" {
		return errMissingBespokePackageName
	}
	if !matches(pkgName) {
		return fmt.Errorf("%w %q; apko packages %v (apko will fail with 'not in indexes' at publish time)", errBespokePackageMismatch, pkgName, packages)
	}
	return nil
}
