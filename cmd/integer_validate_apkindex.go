package cmd

import (
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/verity-org/verity/internal/integer/apkindex"
	intconfig "github.com/verity-org/verity/internal/integer/config"
)

func validateDeclaredVersionsAgainstAPKINDEX(def *intconfig.ImageDef, defPath string, pkgs []apkindex.Package) int {
	resolutionPattern := def.VersionedPackagePattern()
	if resolutionPattern == "" {
		return 0
	}
	if !anyTypeUsesPackage(def, resolutionPattern) {
		return 0
	}

	failures := 0
	for v := range def.Versions {
		if v == "" || v == latestSentinel {
			continue
		}
		resolved := apkindex.ResolveAliasVersion(pkgs, resolutionPattern, v)
		if resolved != v {
			continue
		}
		literal := strings.ReplaceAll(resolutionPattern, "{{version}}", v)
		if hasPackageName(pkgs, literal) {
			continue
		}
		fmt.Fprintf(os.Stderr,
			"FAIL %s: declared version %q is not satisfiable — Wolfi APKINDEX has no package "+
				"named %q, and no %q.<minor>-style package (e.g. %q.0, %q.1) is published either "+
				"(apko publish would fail with `nothing provides %q`); "+
				"declare a specific minor that Wolfi actually publishes\n",
			defPath, v, literal, literal, literal, literal, literal)
		failures++
	}
	return failures
}

func validateAgainstAPKINDEX(def *intconfig.ImageDef, defPath string, pkgs []apkindex.Package) (int, bool) {
	if len(pkgs) == 0 {
		return 0, false
	}
	if found := apkindex.DiscoverVersions(pkgs, def.Upstream.Package); len(found) == 0 {
		fmt.Fprintf(os.Stderr, "FAIL %s: upstream package %q not found in APKINDEX\n",
			defPath, def.Upstream.Package)
		return 1, true
	}
	versionFails := validateDeclaredVersionsAgainstAPKINDEX(def, defPath, pkgs)
	if versionFails > 0 {
		return versionFails, true
	}
	return 0, false
}

func anyTypeUsesPackage(def *intconfig.ImageDef, pkg string) bool {
	for typeName := range def.Types {
		tmpl := def.Types[typeName]
		if slices.Contains(tmpl.Packages, pkg) {
			return true
		}
	}
	return false
}

func hasPackageName(pkgs []apkindex.Package, n string) bool {
	for _, pkg := range pkgs {
		if pkg.Name == n {
			return true
		}
	}
	return false
}
