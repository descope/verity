package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/urfave/cli/v3"
	"gopkg.in/yaml.v3"

	"github.com/verity-org/verity/internal/integer/apkindex"
	intconfig "github.com/verity-org/verity/internal/integer/config"
)

var errIntegerValidationFailed = errors.New("validation failed")

// bespokePkgFile is the minimal subset of a melange yaml needed to verify the
// package name. Other fields (environment, pipeline, etc.) are intentionally
// ignored — this lives in /cmd to avoid bringing melange's own schema into
// the verity tree.
type bespokePkgFile struct {
	Package struct {
		Name string `yaml:"name"`
	} `yaml:"package"`
}

var integerValidateCmd = &cli.Command{
	Name:  "validate",
	Usage: "Schema-validate all image configs in images/",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "config",
			Aliases: []string{"c"},
			Usage:   "Path to integer.yaml",
			Value:   "integer.yaml",
		},
		&cli.StringFlag{
			Name:  "images-dir",
			Usage: "Path to the images/ directory",
			Value: "images",
		},
		&cli.StringFlag{
			Name:  "bespoke-dir",
			Usage: "Path to the packages/bespoke/ directory; bespoke melange yamls referenced from images/ are cross-checked here",
			Value: filepath.Join("packages", "bespoke"),
		},
		&cli.StringFlag{
			Name:  "apkindex-url",
			Usage: "Wolfi APKINDEX URL; when set, verifies upstream packages exist (skipped if empty)",
			Value: "",
		},
		&cli.StringFlag{
			Name:  "cache-dir",
			Usage: "Directory for caching APKINDEX data",
			Value: os.TempDir(),
		},
	},
	Action: runIntegerValidate,
}

func runIntegerValidate(_ context.Context, cmd *cli.Command) error {
	cfgPath := cmd.String("config")
	imagesDir := cmd.String("images-dir")
	bespokeDir := cmd.String("bespoke-dir")

	// Stat bespokeDir once. When the directory is missing, validateBespokeRefs
	// emits a single summary FAIL per affected def instead of N per-type
	// ENOENTs (which previously also double-counted with reportOrphanBespoke's
	// own summary). When the directory exists, the per-file behavior is
	// unchanged.
	bespokeDirExists := isExistingDir(bespokeDir)

	var pkgs []apkindex.Package
	if url := cmd.String("apkindex-url"); url != "" {
		var err error
		pkgs, err = apkindex.Fetch(url, cmd.String("cache-dir"), apkindex.DefaultCacheMaxAge)
		if err != nil {
			return fmt.Errorf("fetching APKINDEX: %w", err)
		}
	}

	failures := 0

	if _, err := intconfig.LoadConfig(cfgPath); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL %s: %v\n", cfgPath, err)
		failures++
	} else {
		fmt.Fprintf(os.Stdout, "OK   %s\n", cfgPath)
	}

	entries, err := os.ReadDir(imagesDir)
	if err != nil {
		return fmt.Errorf("reading images directory: %w", err)
	}

	// Track which bespoke files are referenced so we can flag orphans below.
	referencedBespoke := map[string]string{} // bespoke filename → image yaml path

	checked := 0
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != yamlExt {
			continue
		}

		defPath := filepath.Join(imagesDir, entry.Name())
		def, err := intconfig.LoadImage(defPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FAIL %s: %v\n", defPath, err)
			failures++
			continue
		}
		if err := intconfig.Validate(def); err != nil {
			fmt.Fprintf(os.Stderr, "FAIL %s: %v\n", defPath, err)
			failures++
			continue
		}

		apkFails, skip := validateAgainstAPKINDEX(def, defPath, pkgs)
		failures += apkFails
		if skip {
			continue
		}

		// Bespoke cross-checks: every type that declares melange.bespoke must
		// reference a packages/bespoke/<file>.yaml whose package.name appears
		// in the type's apko packages: list. Without this guard, a typo (or
		// a forgotten image-yaml entry) silently produces an apk that apko
		// can't resolve at publish time — exactly the failure mode of #297.
		bespokeFails := validateBespokeRefs(def, defPath, bespokeDir, referencedBespoke, bespokeDirExists)
		if bespokeFails > 0 {
			failures += bespokeFails
			continue
		}

		fmt.Fprintf(os.Stdout, "OK   %s (%d types, %d declared versions)\n",
			defPath, len(def.Types), len(def.Versions))
		checked++
	}

	// Detect orphan bespoke yamls: files in packages/bespoke/ that no image
	// references. These are dead weight and usually indicate someone added a
	// bespoke package but forgot to wire it into images/.
	if orphanFailures := reportOrphanBespoke(bespokeDir, referencedBespoke, bespokeDirExists); orphanFailures > 0 {
		failures += orphanFailures
	}

	if failures > 0 {
		return fmt.Errorf("%d error(s): %w", failures, errIntegerValidationFailed)
	}

	fmt.Fprintf(os.Stdout, "\nAll configs valid (%d images checked)\n", checked)
	return nil
}

// validateBespokeRefs verifies, for every type in def that declares
// melange.bespoke, that the referenced packages/bespoke/<file>.yaml exists
// and that its package.name is one of the entries in the type's apko
// packages: list. Each newly seen bespoke filename is recorded in referenced
// so the caller can later detect orphans. Returns the number of failures
// (and prints a FAIL line per failure to stderr).
//
// When dirExists is false, per-file reads are skipped and a single summary
// FAIL is emitted per affected def. This avoids two pathologies in the
// previous implementation: (1) N noisy "reading: open ...: no such file or
// directory" FAILs per type when the dir is absent, and (2) double-counting
// with reportOrphanBespoke's own summary failure.
func validateBespokeRefs(def *intconfig.ImageDef, defPath, bespokeDir string, referenced map[string]string, dirExists bool) int {
	failures := 0
	bespokeRefs := 0
	for typeName, tmpl := range def.Types {
		if tmpl.Melange == nil || tmpl.Melange.Bespoke == "" {
			continue
		}
		bespokeFile := tmpl.Melange.Bespoke
		referenced[bespokeFile] = defPath
		bespokeRefs++

		// Defer the per-file reads to the post-loop summary when the
		// entire bespoke directory is missing — see the function comment.
		if !dirExists {
			continue
		}

		bespokePath := filepath.Join(bespokeDir, bespokeFile)
		pkgName, berr := readBespokePackageName(bespokePath)
		if berr != nil {
			fmt.Fprintf(os.Stderr, "FAIL %s type %q: bespoke %s: %v\n",
				defPath, typeName, bespokeFile, berr)
			failures++
			continue
		}
		if pkgName == "" {
			fmt.Fprintf(os.Stderr, "FAIL %s type %q: bespoke %s: missing package.name\n",
				defPath, typeName, bespokeFile)
			failures++
			continue
		}
		if !slices.Contains(tmpl.Packages, pkgName) {
			fmt.Fprintf(os.Stderr,
				"FAIL %s type %q: bespoke package.name %q not in apko packages: %v "+
					"(apko will fail with 'not in indexes' at publish time)\n",
				defPath, typeName, pkgName, tmpl.Packages)
			failures++
			continue
		}
	}

	if !dirExists && bespokeRefs > 0 {
		fmt.Fprintf(os.Stderr,
			"FAIL %s: bespoke-dir %s does not exist but %d type(s) reference bespoke files\n",
			defPath, bespokeDir, bespokeRefs)
		failures++
	}

	return failures
}

// validateDeclaredVersionsAgainstAPKINDEX flags declared `versions: X` keys
// that the Wolfi APKINDEX cannot satisfy. For versioned upstream patterns
// every declared X must resolve to either a literal "<pkg>-X" or some
// aliasable "<pkg>-X.<minor>" — otherwise the apko config rendered for that
// dispatch will fail at publish time with `nothing provides "<pkg>-X"`.
//
// Skips:
//   - Unversioned upstream patterns (no "{{version}}"). The image-level
//     existence check handles them; declared versions are usually just
//     "latest" and don't need APKINDEX presence.
//   - Types whose template is purely bespoke (declares melange.bespoke and
//     does not include the upstream apk in packages:). Bespoke builds
//     supply the apk locally so APKINDEX presence isn't required.
//
// When at least one type in the def DOES depend on the upstream apk
// (the common case — every default/dev/fips type that doesn't override
// packages:), the declared version must be APKINDEX-resolvable.
func validateDeclaredVersionsAgainstAPKINDEX(def *intconfig.ImageDef, defPath string, pkgs []apkindex.Package) int {
	// Pick the actual package pattern apk solving will use. For most
	// images this is Upstream.Package; for the erlang/haproxy shape
	// (unversioned upstream.package + versioned type packages),
	// VersionedPackagePattern walks types[*].packages to find the
	// constraint that actually matters. Bail when no version template
	// is reachable — purely-bespoke or fully-pinned images don't need
	// the per-version guard.
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
		// ResolveAliasVersion returns v unchanged when there is neither a
		// literal nor an alias match. To distinguish "literal exists" from
		// "unresolvable", check the literal one more time when resolved == v.
		if resolved != v {
			continue // alias matched a "<v>.<minor>" stem — render-time fix kicks in
		}
		literal := strings.ReplaceAll(resolutionPattern, "{{version}}", v)
		if hasPackageName(pkgs, literal) {
			continue // literal matched
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

// validateAgainstAPKINDEX runs the two APKINDEX-dependent guards for one
// image def: the image-level "upstream package present" check and the
// per-declared-version satisfiability check. Returns the failure count and
// a skip flag indicating whether the caller should `continue` to the next
// image (mirrors the structure of the inline block this replaced, kept here
// to keep runIntegerValidate's cyclomatic complexity below the lint cap).
//
// When pkgs is empty (no --apkindex-url supplied), both guards are no-ops
// and the function returns (0, false).
func validateAgainstAPKINDEX(def *intconfig.ImageDef, defPath string, pkgs []apkindex.Package) (int, bool) {
	if len(pkgs) == 0 {
		return 0, false
	}
	if found := apkindex.DiscoverVersions(pkgs, def.Upstream.Package); len(found) == 0 {
		fmt.Fprintf(os.Stderr, "FAIL %s: upstream package %q not found in APKINDEX\n",
			defPath, def.Upstream.Package)
		return 1, true
	}
	// Per-declared-version guard. For versioned upstream patterns every
	// "versions: X" must map to either a literal "<pkg>-X" in the
	// APKINDEX or some "<pkg>-X.<minor>" the renderer can alias to.
	// Without this guard, declarations like `kyverno: { "1": {} }` slip
	// through (because "kyverno-1.17" makes the image-level check pass)
	// and then blow up at apko publish time with
	// `nothing provides "kyverno-1"` — the chronic Integer Build Image
	// failure mode.
	//
	// Skip purely-bespoke types (no upstream apk dependency) and
	// unversioned upstream patterns, which are already covered by the
	// image-level check above.
	versionFails := validateDeclaredVersionsAgainstAPKINDEX(def, defPath, pkgs)
	if versionFails > 0 {
		return versionFails, true
	}
	return 0, false
}

// anyTypeUsesPackage reports whether at least one type in def lists pkg
// in its packages: constraint. Used by the per-version validate guard to
// skip images whose types don't actually depend on the resolution pattern
// at hand: bespoke-only types and images where upstream.package is
// declared but never referenced by a type.
func anyTypeUsesPackage(def *intconfig.ImageDef, pkg string) bool {
	for _, tmpl := range def.Types {
		if slices.Contains(tmpl.Packages, pkg) {
			return true
		}
	}
	return false
}

// hasPackageName reports whether any pkg in pkgs has the exact name n.
func hasPackageName(pkgs []apkindex.Package, n string) bool {
	for _, pkg := range pkgs {
		if pkg.Name == n {
			return true
		}
	}
	return false
}

// isExistingDir returns true iff path is a non-empty string and refers to
// an existing directory. Used to decide once, up front, whether the
// bespoke-dir is present so callers can skip per-file reads when it isn't.
func isExistingDir(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// readBespokePackageName parses a melange yaml and returns its package.name.
// Returns ("", err) on read/parse failures, ("", nil) if the field is absent.
func readBespokePackageName(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading: %w", err)
	}
	var f bespokePkgFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return "", fmt.Errorf("parsing: %w", err)
	}
	return f.Package.Name, nil
}

// reportOrphanBespoke lists the immediate entries in bespokeDir and prints
// a FAIL line for every top-level *.yaml file that no image yaml references.
// Returns the failure count.
//
// Skips entirely when bespokeDir does not exist (dirExists is false): the
// "missing-but-referenced" case is already counted by validateBespokeRefs's
// per-def summary, and an orphan check is meaningless against a missing dir.
func reportOrphanBespoke(bespokeDir string, referenced map[string]string, dirExists bool) int {
	if !dirExists {
		return 0
	}
	entries, err := os.ReadDir(bespokeDir)
	if err != nil {
		// dirExists was true at startup, so a ReadDir error here is a
		// race or a permissions issue — treat as a hard failure.
		fmt.Fprintf(os.Stderr, "FAIL bespoke-dir %s: %v\n", bespokeDir, err)
		return 1
	}
	failures := 0
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != yamlExt {
			continue
		}
		if _, ok := referenced[e.Name()]; !ok {
			fmt.Fprintf(os.Stderr,
				"FAIL %s: orphan bespoke yaml — no image in images/ references it via melange.bespoke\n",
				filepath.Join(bespokeDir, e.Name()))
			failures++
		}
	}
	return failures
}
